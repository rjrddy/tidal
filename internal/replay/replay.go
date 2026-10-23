// Package replay runs deterministic counterfactual caches against public/synthetic keys.
package replay

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/rjrddy/tidal/internal/cache"
	"io"
	"math/rand"
	"os"
	"runtime"
	"time"
)

type Event struct {
	Key      string
	Write    string
	Announce bool
}
type Sample struct {
	At     int     `json:"at"`
	Hits   float64 `json:"hits"`
	Origin uint64  `json:"origin"`
	Stale  float64 `json:"stale"`
	Policy string  `json:"policy"`
}
type Row struct {
	Trace            string         `json:"trace"`
	Mode             string         `json:"mode"`
	Requests         int            `json:"requests"`
	Hits             uint64         `json:"hits"`
	HitRate          float64        `json:"hit_rate"`
	Origin           uint64         `json:"origin_calls"`
	OriginPerRequest float64        `json:"origin_per_request"`
	Refreshes        uint64         `json:"refreshes"`
	Stale            uint64         `json:"stale_serves"`
	StaleRate        float64        `json:"stale_rate"`
	Switches         uint64         `json:"switches"`
	ElapsedMS        float64        `json:"elapsed_ms"`
	Samples          []Sample       `json:"samples"`
	Windows          []cache.Window `json:"windows"`
}

func Synthetic(name string, n int, seed int64) []Event {
	r := rand.New(rand.NewSource(seed))
	z := rand.NewZipf(r, 1.2, 1, 2047)
	out := make([]Event, n)
	for i := range out {
		var k int
		switch name {
		case "hotset":
			if r.Intn(10) < 9 {
				k = r.Intn(96)
			} else {
				k = r.Intn(2048)
			}
		case "scan":
			if r.Intn(10) < 3 {
				k = r.Intn(64)
			} else {
				k = 128 + i
			}
		case "shift":
			if i < n/3 {
				k = r.Intn(96)
			} else if i < 2*n/3 {
				k = int(z.Uint64()) + 4096
			} else {
				k = 8192 + r.Intn(96)
			}
		default:
			panic("unknown synthetic trace")
		}
		out[i].Key = fmt.Sprintf("k%d", k)
		if i > 0 && i%73 == 0 {
			out[i].Write = out[i-1].Key
			out[i].Announce = i%5 != 0
		}
	}
	return out
}
func Public(path string, limit int) ([]Event, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != "30e5ec69a7e1a1789f17d788e4aab61146dce68a449fb29779ead89da944e4c9" {
		return nil, fmt.Errorf("public trace hash mismatch")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	data, err := io.ReadAll(io.LimitReader(gz, 4000001))
	if err != nil {
		return nil, err
	}
	if len(data)%4 != 0 || len(data) > 4000000 {
		return nil, fmt.Errorf("invalid trace length")
	}
	n := len(data) / 4
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Event, n)
	for i := range out {
		key := binary.BigEndian.Uint32(data[i*4:])
		out[i].Key = fmt.Sprintf("p%d", key)
		if i > 0 && i%73 == 0 {
			out[i].Write = out[i-1].Key
			out[i].Announce = i%5 != 0
		}
	}
	return out, nil
}
func Config(mode string) cache.Config {
	c := cache.Config{Capacity: 300, Policy: "lru", TTL: 30 * time.Second, MaxTTL: 30 * time.Second, Window: 512, InlineRefresh: true}
	switch mode {
	case "fixed-ttl":
		c.TTL = 5 * time.Second
	case "plain-lru":
	case "lfu":
		c.Policy = "lfu"
	case "tinylfu":
		c.Policy = "tinylfu"
	case "adaptive":
		c.Adaptive = true
		c.EarlyRefresh = true
	default:
		panic("unknown mode")
	}
	return c
}
func Run(name, mode string, events []Event) Row {
	now := time.Unix(0, 0)
	versions := map[string]uint64{}
	c := Config(mode)
	loader := func(_ context.Context, key string) (cache.Value, error) {
		return cache.Value{Key: key, Version: versions[key], Body: "synthetic value", MaxAgeMS: 30000}, nil
	}
	m, err := cache.New(c, loader, func() time.Time { return now })
	if err != nil {
		panic(err)
	}
	row := Row{Trace: name, Mode: mode, Requests: len(events)}
	start := time.Now()
	var blockHits, blockStale uint64
	for i, e := range events {
		now = now.Add(10 * time.Millisecond)
		if e.Write != "" {
			versions[e.Write]++
			if e.Announce {
				m.Invalidate(e.Write)
			}
		}
		result, err := m.Get(context.Background(), e.Key)
		if err != nil {
			panic(err)
		}
		if result.Hit {
			blockHits++
		}
		if result.Value.Version != versions[e.Key] {
			row.Stale++
			blockStale++
		}
		if (i+1)%512 == 0 {
			s := m.Snapshot()
			row.Samples = append(row.Samples, Sample{At: i + 1, Hits: float64(blockHits) / 512, Origin: s.OriginCalls, Stale: float64(blockStale) / 512, Policy: s.Selected})
			blockHits = 0
			blockStale = 0
		}
	}
	s := m.Snapshot()
	row.Hits = s.Hits
	row.HitRate = float64(s.Hits) / float64(len(events))
	row.Origin = s.OriginCalls
	row.OriginPerRequest = float64(s.OriginCalls) / float64(len(events))
	row.Refreshes = s.Refreshes
	row.StaleRate = float64(row.Stale) / float64(len(events))
	row.Switches = s.Switches
	row.Windows = s.Windows
	row.ElapsedMS = float64(time.Since(start).Microseconds()) / 1000
	return row
}
func Campaign(out string) error {
	type report struct {
		Seed  int    `json:"seed"`
		Go    string `json:"go"`
		OS    string `json:"os"`
		Arch  string `json:"arch"`
		Notes string `json:"notes"`
		Rows  []Row  `json:"rows"`
	}
	result := report{Seed: 42, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Notes: "Equal 300-object capacities; 10ms synthetic clock/request; versions mutate every 73 requests with deterministic dropped invalidations. Public Web12 keys only; timestamps/versions/TTLs synthetic. Early refresh inline in replay, asynchronous in HTTP. Origin calls include refresh. Not service throughput; no object-byte sizing."}
	names := []string{"hotset", "scan", "shift", "web12"}
	for _, name := range names {
		var events []Event
		if name == "web12" {
			v, err := Public("data/web12.bin.gz", 0)
			if err != nil {
				return err
			}
			events = v
		} else {
			events = Synthetic(name, 30000, 42)
		}
		for _, mode := range []string{"fixed-ttl", "plain-lru", "lfu", "tinylfu", "adaptive"} {
			row := Run(name, mode, events)
			result.Rows = append(result.Rows, row)
			fmt.Printf("%s %s requests=%d hit=%.4f origin=%d refresh=%d stale=%d switches=%d\n", name, mode, row.Requests, row.HitRate, row.Origin, row.Refreshes, row.Stale, row.Switches)
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(data, '\n'), 0644)
}
