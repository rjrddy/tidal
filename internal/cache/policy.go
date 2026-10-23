// Package cache implements bounded immutable-value caching and policy experiments.
package cache

import (
	"container/list"
	"hash/fnv"
	"math"
	"sort"
	"time"
)

// Value is copied on return. Strings are immutable; no caller-owned mutable payloads.
type Value struct {
	Key      string `json:"key"`
	Version  uint64 `json:"version"`
	Body     string `json:"body"`
	MaxAgeMS int64  `json:"max_age_ms"`
}
type entry struct {
	value           Value
	expires, loaded time.Time
	freq, seq       uint64
	node            *list.Element
}

// Sketch is a bounded count-min frequency estimator with periodic aging.
// It implements TinyLFU-style admission over LRU victims, not W-TinyLFU.
type Sketch struct {
	rows    [4][2048]uint16
	samples uint64
}

func hash(key string) uint64 { h := fnv.New64a(); _, _ = h.Write([]byte(key)); return h.Sum64() }
func (s *Sketch) positions(key string) [4]uint64 {
	h := hash(key)
	return [4]uint64{h % 2048, (h>>11 ^ h*0x9e3779b97f4a7c15) % 2048, (h>>23 ^ h*0xbf58476d1ce4e5b9) % 2048, (h>>37 ^ h*0x94d049bb133111eb) % 2048}
}
func (s *Sketch) Add(key string) {
	for r, p := range s.positions(key) {
		if s.rows[r][p] < math.MaxUint16 {
			s.rows[r][p]++
		}
	}
	s.samples++
	if s.samples >= 20000 {
		for r := range s.rows {
			for c := range s.rows[r] {
				s.rows[r][c] /= 2
			}
		}
		s.samples = 0
	}
}
func (s *Sketch) Count(key string) uint16 {
	n := uint16(math.MaxUint16)
	for r, p := range s.positions(key) {
		if s.rows[r][p] < n {
			n = s.rows[r][p]
		}
	}
	return n
}

type store struct {
	capacity int
	policy   string
	entries  map[string]*entry
	order    *list.List
	sketch   Sketch
	sequence uint64
}

func newStore(capacity int, policy string) *store {
	return &store{capacity: capacity, policy: policy, entries: make(map[string]*entry), order: list.New()}
}
func (s *store) remove(key string) {
	if e := s.entries[key]; e != nil {
		s.order.Remove(e.node)
		delete(s.entries, key)
	}
}
func (s *store) get(key string, now time.Time) (Value, bool, bool) {
	s.sketch.Add(key)
	e := s.entries[key]
	if e == nil {
		return Value{}, false, false
	}
	if !now.Before(e.expires) {
		s.remove(key)
		return Value{}, false, false
	}
	s.sequence++
	e.seq = s.sequence
	if e.freq < math.MaxUint64 {
		e.freq++
	}
	s.order.MoveToFront(e.node)
	hot := s.sketch.Count(key) >= 4
	near := now.Sub(e.loaded)*5 >= e.expires.Sub(e.loaded)*4
	return e.value, true, hot && near
}
func (s *store) put(v Value, now time.Time, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	if e := s.entries[v.Key]; e != nil {
		e.value = v
		e.loaded = now
		e.expires = now.Add(ttl)
		return true
	}
	// Expired objects never displace a fresh candidate through TinyLFU admission.
	for key, e := range s.entries {
		if !now.Before(e.expires) {
			s.remove(key)
		}
	}
	if len(s.entries) >= s.capacity {
		victim := s.order.Back().Value.(string)
		if s.policy == "lfu" {
			best := s.entries[victim]
			for key, e := range s.entries {
				if e.freq < best.freq || e.freq == best.freq && e.seq < best.seq {
					victim = key
					best = e
				}
			}
		}
		if s.policy == "tinylfu" && s.sketch.Count(v.Key) <= s.sketch.Count(victim) {
			return false
		}
		s.remove(victim)
	}
	s.sequence++
	node := s.order.PushFront(v.Key)
	s.entries[v.Key] = &entry{value: v, loaded: now, expires: now.Add(ttl), freq: 1, seq: s.sequence, node: node}
	return true
}
func (s *store) keys() []string {
	keys := make([]string, 0, len(s.entries))
	for k := range s.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
