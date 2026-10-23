package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrInvalidated = errors.New("invalidation crossed origin fetch; retry required")

type Loader func(context.Context, string) (Value, error)
type Config struct {
	Capacity      int
	Policy        string
	Adaptive      bool
	TTL           time.Duration
	Window        int
	EarlyRefresh  bool
	InlineRefresh bool
	MaxTTL        time.Duration
}
type observation struct {
	last time.Time
	ema  time.Duration
}
type flight struct {
	done  chan struct{}
	value Value
	err   error
}
type Window struct {
	At       uint64             `json:"at"`
	Selected string             `json:"selected"`
	Rates    map[string]float64 `json:"rates"`
	Switched bool               `json:"switched"`
}
type Stats struct {
	Requests         uint64   `json:"requests"`
	Hits             uint64   `json:"hits"`
	OriginCalls      uint64   `json:"origin_calls"`
	Refreshes        uint64   `json:"refreshes"`
	Errors           uint64   `json:"errors"`
	InvalidatedLoads uint64   `json:"invalidated_loads"`
	Switches         uint64   `json:"switches"`
	Selected         string   `json:"selected"`
	Resident         int      `json:"resident"`
	Inflight         int      `json:"inflight"`
	Windows          []Window `json:"windows"`
}
type Result struct {
	Value   Value  `json:"value"`
	Hit     bool   `json:"hit"`
	Refresh bool   `json:"refresh_eligible"`
	Policy  string `json:"policy"`
}
type Manager struct {
	mu           sync.Mutex
	config       Config
	active       *store
	shadow       map[string]*store
	score        map[string]uint64
	observations map[string]observation
	flights      map[string]*flight
	epoch        uint64
	stats        Stats
	challenger   string
	streak       int
	lastSwitch   uint64
	loader       Loader
	clock        func() time.Time
	slots        chan struct{}
}

func New(c Config, loader Loader, clock func() time.Time) (*Manager, error) {
	if c.Capacity < 1 || c.Capacity > 10000 || c.Window < 16 || c.TTL <= 0 || c.MaxTTL <= 0 || c.MaxTTL > time.Hour || c.TTL > c.MaxTTL || loader == nil || clock == nil {
		return nil, errors.New("invalid cache configuration")
	}
	if c.Policy != "lru" && c.Policy != "lfu" && c.Policy != "tinylfu" {
		return nil, errors.New("unknown cache policy")
	}
	m := &Manager{config: c, active: newStore(c.Capacity, c.Policy), loader: loader, clock: clock, shadow: map[string]*store{}, score: map[string]uint64{}, observations: map[string]observation{}, flights: map[string]*flight{}, slots: make(chan struct{}, 4)}
	m.stats.Selected = c.Policy
	for _, p := range []string{"lru", "lfu", "tinylfu"} {
		m.shadow[p] = newStore(c.Capacity, p)
	}
	return m, nil
}
func (m *Manager) ttl(key string, maxAge time.Duration) time.Duration {
	ttl := m.config.TTL
	if m.config.Adaptive {
		if o, ok := m.observations[key]; ok && o.ema > 0 {
			ttl = 4 * o.ema
		}
	}
	if ttl < 50*time.Millisecond {
		ttl = 50 * time.Millisecond
	}
	if ttl > m.config.MaxTTL {
		ttl = m.config.MaxTTL
	}
	if ttl > maxAge {
		ttl = maxAge
	}
	return ttl
}
func (m *Manager) observe(key string, now time.Time) {
	o := m.observations[key]
	if !o.last.IsZero() {
		gap := now.Sub(o.last)
		if gap < 0 {
			gap = 0
		}
		if o.ema == 0 {
			o.ema = gap
		} else {
			o.ema = (3*o.ema + gap) / 4
		}
	}
	o.last = now
	// Bounded metadata, no arbitrary-key tombstone or interval map growth.
	if len(m.observations) >= 4*m.config.Capacity {
		clear(m.observations)
	}
	m.observations[key] = o
	for p, s := range m.shadow {
		_, hit, _ := s.get(key, now)
		if hit {
			m.score[p]++
		} else {
			s.put(Value{Key: key}, now, m.config.TTL)
		}
	}
	if m.stats.Requests%uint64(m.config.Window) != 0 {
		return
	}
	rates := map[string]float64{}
	best := m.active.policy
	for _, p := range []string{"lru", "lfu", "tinylfu"} {
		rates[p] = float64(m.score[p]) / float64(m.config.Window)
		if m.score[p] > m.score[best] {
			best = p
		}
	}
	switched := false
	if m.config.Adaptive && best != m.active.policy && rates[best] > rates[m.active.policy]+.05 {
		if m.challenger == best {
			m.streak++
		} else {
			m.challenger = best
			m.streak = 1
		}
		if m.streak >= 2 && m.stats.Requests-m.lastSwitch >= uint64(2*m.config.Window) {
			m.active.policy = best
			m.stats.Selected = best
			m.stats.Switches++
			m.lastSwitch = m.stats.Requests
			switched = true
			m.streak = 0
		}
	} else {
		m.streak = 0
		m.challenger = ""
	}
	m.stats.Windows = append(m.stats.Windows, Window{At: m.stats.Requests, Selected: m.active.policy, Rates: rates, Switched: switched})
	if len(m.stats.Windows) > 200 {
		m.stats.Windows = m.stats.Windows[len(m.stats.Windows)-200:]
	}
	clear(m.score)
}
func validate(v Value, key string) error {
	if v.Key != key || v.MaxAgeMS < 0 || v.MaxAgeMS > 3600000 || len(v.Body) > 65536 {
		return errors.New("origin response violates cache contract")
	}
	return nil
}
func (m *Manager) load(ctx context.Context, key string, refresh bool) (Value, error) {
	m.mu.Lock()
	// Recheck under the flight lock: another request may have filled after our miss.
	if e := m.active.entries[key]; e != nil && m.clock().Before(e.expires) {
		if !refresh || m.clock().Sub(e.loaded)*5 < e.expires.Sub(e.loaded)*4 {
			v := e.value
			m.mu.Unlock()
			return v, nil
		}
	}

	if f := m.flights[key]; f != nil {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Value{}, ctx.Err()
		case <-f.done:
			return f.value, f.err
		}
	}
	if len(m.flights) >= 32 {
		m.mu.Unlock()
		return Value{}, errors.New("origin concurrency budget exhausted")
	}
	f := &flight{done: make(chan struct{})}
	m.flights[key] = f
	epoch := m.epoch
	m.stats.OriginCalls++
	if refresh {
		m.stats.Refreshes++
	}
	m.mu.Unlock()
	value, err := m.loader(ctx, key)
	if err == nil {
		err = validate(value, key)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil && epoch != m.epoch {
		err = ErrInvalidated
		m.stats.InvalidatedLoads++
	}
	if err == nil {
		m.active.put(value, m.clock(), m.ttl(key, time.Duration(value.MaxAgeMS)*time.Millisecond))
	} else {
		m.stats.Errors++
	}
	f.value = value
	f.err = err
	delete(m.flights, key)
	close(f.done)
	return value, err
}
func (m *Manager) Get(ctx context.Context, key string) (Result, error) {
	if len(key) == 0 || len(key) > 128 {
		return Result{}, errors.New("invalid key length")
	}
	for attempt := 0; attempt < 3; attempt++ {
		m.mu.Lock()
		now := m.clock()
		if attempt == 0 {
			m.stats.Requests++
			m.observe(key, now)
		}
		v, hit, near := m.active.get(key, now)
		policy := m.active.policy
		early := hit && near && m.config.EarlyRefresh
		if hit {
			m.stats.Hits++
		}
		m.mu.Unlock()
		if hit {
			if early {
				if m.config.InlineRefresh {
					_, _ = m.load(ctx, key, true)
				} else {
					select {
					case m.slots <- struct{}{}:
						go func() {
							defer func() { <-m.slots }()
							refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
							defer cancel()
							_, _ = m.load(refreshCtx, key, true)
						}()
					default:
					}
				}
			}
			return Result{Value: v, Hit: true, Refresh: early, Policy: policy}, nil
		}
		loaded, err := m.load(ctx, key, false)
		if errors.Is(err, ErrInvalidated) {
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("origin load: %w", err)
		}
		return Result{Value: loaded, Policy: policy}, nil
	}
	return Result{}, ErrInvalidated
}
func (m *Manager) Invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.epoch++
	m.active.remove(key)
	for _, s := range m.shadow {
		s.remove(key)
	}
}
func (m *Manager) Snapshot() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stats
	s.Resident = len(m.active.entries)
	s.Inflight = len(m.flights)
	s.Windows = append([]Window{}, m.stats.Windows...)
	return s
}
func (m *Manager) Keys() []string { m.mu.Lock(); defer m.mu.Unlock(); return m.active.keys() }

// Safety: max-age is authoritative; adaptive TTL never extends it. It is not an invalidation oracle.
