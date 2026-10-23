package replay

import (
	"reflect"
	"testing"
)

func TestDeterministicAndOriginAccounting(t *testing.T) {
	for _, name := range []string{"hotset", "scan", "shift"} {
		events := Synthetic(name, 3000, 42)
		for _, mode := range []string{"fixed-ttl", "plain-lru", "adaptive"} {
			a := Run(name, mode, events)
			b := Run(name, mode, events)
			a.ElapsedMS = 0
			b.ElapsedMS = 0
			if !reflect.DeepEqual(a, b) {
				t.Fatal("non-deterministic replay", name, mode)
			}
			if a.Origin != uint64(a.Requests)-a.Hits+a.Refreshes {
				t.Fatal("origin accounting", a)
			}
			if a.Stale > uint64(a.Requests) || a.HitRate < 0 || a.HitRate > 1 {
				t.Fatal("metrics")
			}
		}
	}
}
func TestUnannouncedUpdatesAreNotFreshnessOracle(t *testing.T) {
	events := []Event{}
	for i := 0; i < 100; i++ {
		e := Event{Key: "a"}
		if i == 20 {
			e.Write = "a"
		}
		events = append(events, e)
	}
	row := Run("negative-control", "plain-lru", events)
	if row.Stale == 0 {
		t.Fatal("negative control did not expose stale serves")
	}
	t.Logf("caught %d stale serves from deliberately unannounced origin write", row.Stale)
}
