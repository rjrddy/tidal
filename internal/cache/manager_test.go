package cache

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func config() Config {
	return Config{Capacity: 4, Policy: "lru", TTL: time.Second, MaxTTL: time.Second, Window: 16}
}
func TestPolicies(t *testing.T) {
	for _, p := range []string{"lru", "lfu", "tinylfu"} {
		t.Run(p, func(t *testing.T) {
			s := newStore(3, p)
			now := time.Unix(1, 0)
			for _, k := range []string{"a", "b", "c"} {
				s.get(k, now)
				s.put(Value{Key: k}, now, time.Second)
			}
			for i := 0; i < 20; i++ {
				s.get("a", now)
			}
			s.get("d", now)
			s.put(Value{Key: "d"}, now, time.Second)
			if _, ok := s.entries["a"]; !ok {
				t.Fatal("hot entry evicted")
			}
			if len(s.entries) > 3 {
				t.Fatal("capacity")
			}
			if _, ok, _ := s.get("a", now.Add(time.Second)); ok {
				t.Fatal("TTL boundary served")
			}
		})
	}
}
func TestRandomizedLRUReference(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	s := newStore(7, "lru")
	reference := []string{}
	now := time.Unix(1, 0)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("k%d", r.Intn(30))
		pos := -1
		for j, k := range reference {
			if k == key {
				pos = j
			}
		}
		_, hit, _ := s.get(key, now)
		if hit != (pos >= 0) {
			t.Fatalf("step %d reference mismatch", i)
		}
		if pos >= 0 {
			reference = append(reference[:pos], reference[pos+1:]...)
		}
		reference = append([]string{key}, reference...)
		if len(reference) > 7 {
			reference = reference[:7]
		}
		if !hit {
			s.put(Value{Key: key}, now, time.Hour)
		}
		if len(s.entries) != len(reference) {
			t.Fatal("size mismatch")
		}
		for _, k := range reference {
			if s.entries[k] == nil {
				t.Fatal("resident mismatch")
			}
		}
	}
	t.Log("10000 seeded requests match independent ordered-list LRU reference")
}
func TestCoalescingAndInvalidationFence(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(ctx context.Context, key string) (Value, error) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Value{}, ctx.Err()
			}
		}
		return Value{Key: key, Version: uint64(n), MaxAgeMS: 1000}, nil
	}
	m, _ := New(config(), loader, time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := m.Get(context.Background(), "a")
			if e != nil || v.Value.Version < 2 {
				t.Errorf("stale fill escaped: %+v %v", v, e)
			}
		}()
	}
	<-started
	m.Invalidate("a")
	close(release)
	wg.Wait()
	if m.Snapshot().InvalidatedLoads != 1 {
		t.Fatal("missing invalidation fence")
	}
	if calls.Load() != 2 {
		t.Fatalf("coalescing calls=%d", calls.Load())
	}
	t.Log("caught old in-flight response; invalidated load rejected and retried")
}
func TestFailureCancellationAndNoStaleFallback(t *testing.T) {
	now := time.Unix(1, 0)
	fail := false
	loader := func(ctx context.Context, key string) (Value, error) {
		if fail {
			return Value{}, errors.New("origin unavailable")
		}
		return Value{Key: key, Version: 1, MaxAgeMS: 1000}, nil
	}
	m, _ := New(config(), loader, func() time.Time { return now })
	if _, e := m.Get(context.Background(), "x"); e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Second)
	fail = true
	if _, e := m.Get(context.Background(), "x"); e == nil {
		t.Fatal("expired served on origin failure")
	}
	if m.Snapshot().Errors != 1 {
		t.Fatal("error metric")
	}
	started, release := make(chan struct{}), make(chan struct{})
	m, _ = New(config(), func(_ context.Context, k string) (Value, error) {
		close(started)
		<-release
		return Value{Key: k, MaxAgeMS: 1000}, nil
	}, time.Now)
	done := make(chan struct{})
	go func() { defer close(done); _, _ = m.Get(context.Background(), "x") }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := m.Get(ctx, "x"); !errors.Is(e, context.Canceled) {
		t.Fatalf("waiter cancellation %v", e)
	}
	close(release)
	<-done
}
func TestTTLBoundAndRefresh(t *testing.T) {
	now := time.Unix(1, 0)
	var calls int
	c := config()
	c.Adaptive = true
	c.EarlyRefresh = true
	c.InlineRefresh = true
	m, _ := New(c, func(_ context.Context, k string) (Value, error) {
		calls++
		return Value{Key: k, Version: uint64(calls), MaxAgeMS: 100}, nil
	}, func() time.Time { return now })
	for i := 0; i < 10; i++ {
		_, e := m.Get(context.Background(), "a")
		if e != nil {
			t.Fatal(e)
		}
		now = now.Add(20 * time.Millisecond)
	}
	s := m.Snapshot()
	if s.Refreshes == 0 {
		t.Fatal("no early refresh")
	}
	m.mu.Lock()
	e := m.active.entries["a"]
	if e.expires.Sub(e.loaded) > 100*time.Millisecond {
		t.Fatal("TTL exceeded origin max-age")
	}
	m.mu.Unlock()
	t.Logf("refreshes=%d origin=%d", s.Refreshes, s.OriginCalls)
}
func TestSketchAgingAndBounds(t *testing.T) {
	var s Sketch
	for i := 0; i < 20000; i++ {
		s.Add("a")
	}
	if s.Count("a") != 10000 {
		t.Fatal("aging")
	}
	m, _ := New(config(), func(_ context.Context, k string) (Value, error) { return Value{Key: k, MaxAgeMS: 1000}, nil }, time.Now)
	for i := 0; i < 10000; i++ {
		_, e := m.Get(context.Background(), fmt.Sprint(i))
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(m.observations) > 4*m.config.Capacity || len(m.active.entries) > m.config.Capacity || len(m.stats.Windows) > 200 {
		t.Fatal("unbounded state")
	}
}
func TestRejectInvalidConfigAndOrigin(t *testing.T) {
	if _, e := New(Config{}, nil, nil); e == nil {
		t.Fatal("invalid config")
	}
	m, _ := New(config(), func(_ context.Context, k string) (Value, error) { return Value{Key: "wrong", MaxAgeMS: 1000}, nil }, time.Now)
	if _, e := m.Get(context.Background(), "a"); e == nil {
		t.Fatal("bad origin accepted")
	}
}
func BenchmarkHit(b *testing.B) {
	c := config()
	m, _ := New(c, func(_ context.Context, k string) (Value, error) { return Value{Key: k, MaxAgeMS: 1000}, nil }, time.Now)
	_, _ = m.Get(context.Background(), "a")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get(context.Background(), "a")
	}
}
