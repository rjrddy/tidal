// Package service exposes a local synthetic origin and the real cached HTTP path.
package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rjrddy/tidal/internal/cache"
	"github.com/rjrddy/tidal/internal/replay"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var keyPattern = regexp.MustCompile(`^[kp][0-9]{1,5}$`)

type Origin struct {
	mu       sync.Mutex
	versions map[string]uint64
	fail     atomic.Bool
	calls    atomic.Uint64
}

func (o *Origin) Version(k string) uint64 { o.mu.Lock(); defer o.mu.Unlock(); return o.versions[k] }
func (o *Origin) Write(k string) uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.versions[k]++
	return o.versions[k]
}
func (o *Origin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.calls.Add(1)
	if o.fail.Load() {
		http.Error(w, "synthetic origin failure", 503)
		return
	}
	k := r.URL.Query().Get("key")
	if !keyPattern.MatchString(k) {
		http.Error(w, "invalid key", 400)
		return
	}
	select {
	case <-time.After(2 * time.Millisecond):
	case <-r.Context().Done():
		return
	}
	write(w, 200, cache.Value{Key: k, Version: o.Version(k), Body: "synthetic response for " + k, MaxAgeMS: 1200})
}

type App struct {
	Cache        *cache.Manager
	Origin       *Origin
	originServer *http.Server
	token        string
	web          string
	driveMu      sync.Mutex
	slots        chan struct{}
	stale        atomic.Uint64
	driveIndex   int
	client       *http.Client
	originURL    string
}

func New(token, web string) (*App, error) {
	o := &Origin{versions: map[string]uint64{}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: o, ReadHeaderTimeout: time.Second, WriteTimeout: 3 * time.Second}
	go func() { _ = server.Serve(listener) }()
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + listener.Addr().String()
	loader := func(ctx context.Context, k string) (cache.Value, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url+"/?key="+k, nil)
		if err != nil {
			return cache.Value{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return cache.Value{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return cache.Value{}, fmt.Errorf("origin status %d", resp.StatusCode)
		}
		var v cache.Value
		decoder := json.NewDecoder(io.LimitReader(resp.Body, 65537))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&v); err != nil {
			return cache.Value{}, err
		}
		return v, nil
	}
	c := replay.Config("adaptive")
	c.InlineRefresh = false
	c.TTL = 1200 * time.Millisecond
	c.MaxTTL = 1200 * time.Millisecond
	c.Window = 256
	c.Capacity = 96
	m, err := cache.New(c, loader, time.Now)
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	return &App{Cache: m, Origin: o, originServer: server, token: token, web: web, slots: make(chan struct{}, 16), client: client, originURL: url}, nil
}
func (a *App) Close() { _ = a.originServer.Close(); a.client.CloseIdleConnections() }
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func body(w http.ResponseWriter, r *http.Request, target any) error {
	if r.Method != "POST" {
		return errors.New("POST required")
	}
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return errors.New("application/json required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return errors.New("single JSON object required")
	}
	return nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.URL.Path == "/health" {
		write(w, 200, map[string]string{"status": "ready"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != "GET" {
			http.Error(w, "method", 405)
			return
		}
		http.FileServer(http.Dir(a.web)).ServeHTTP(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.token)) != 1 {
		write(w, 401, map[string]string{"error": "bearer required"})
		return
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		write(w, 429, map[string]string{"error": "request budget exhausted"})
		return
	}
	defer log.Print(`{"event":"api","path":"` + r.URL.Path + `"}`)
	fail := func(code int, err error) { write(w, code, map[string]string{"error": err.Error()}) }
	switch r.URL.Path {
	case "/api/state":
		if r.Method != "GET" {
			fail(405, errors.New("GET required"))
			return
		}
		write(w, 200, map[string]any{"cache": a.Cache.Snapshot(), "origin_calls": a.Origin.calls.Load(), "origin_failing": a.Origin.fail.Load(), "stale_observed": a.stale.Load(), "keys": a.Cache.Keys(), "capacity": 96, "scope": "live local HTTP origin; stale observed using synthetic origin oracle"})
	case "/api/get":
		if r.Method != "GET" {
			fail(405, errors.New("GET required"))
			return
		}
		k := r.URL.Query().Get("key")
		if !keyPattern.MatchString(k) {
			fail(400, errors.New("key must be k/p followed by digits"))
			return
		}
		start := time.Now()
		v, e := a.Cache.Get(r.Context(), k)
		if e != nil {
			fail(502, e)
			return
		}
		stale := v.Value.Version != a.Origin.Version(k)
		if stale {
			a.stale.Add(1)
		}
		write(w, 200, map[string]any{"result": v, "stale_observed": stale, "elapsed_ms": float64(time.Since(start).Microseconds()) / 1000})
	case "/api/mutate":
		var b struct {
			Key        string `json:"key"`
			Invalidate bool   `json:"invalidate"`
		}
		if e := body(w, r, &b); e != nil {
			fail(400, e)
			return
		}
		if !keyPattern.MatchString(b.Key) {
			fail(400, errors.New("invalid key"))
			return
		}
		version := a.Origin.Write(b.Key)
		if b.Invalidate {
			a.Cache.Invalidate(b.Key)
		}
		write(w, 200, map[string]any{"version": version, "invalidated": b.Invalidate})
	case "/api/fault":
		var b struct {
			Fail bool `json:"fail"`
		}
		if e := body(w, r, &b); e != nil {
			fail(400, e)
			return
		}
		a.Origin.fail.Store(b.Fail)
		write(w, 200, map[string]bool{"fail": b.Fail})
	case "/api/drive":
		var b struct {
			Trace string `json:"trace"`
			Count int    `json:"count"`
		}
		if e := body(w, r, &b); e != nil {
			fail(400, e)
			return
		}
		if b.Trace != "hotset" && b.Trace != "scan" && b.Trace != "shift" || b.Count < 1 || b.Count > 2048 {
			fail(400, errors.New("invalid trace or count"))
			return
		}
		if !a.driveMu.TryLock() {
			fail(409, errors.New("traffic driver already running"))
			return
		}
		defer a.driveMu.Unlock()
		events := replay.Synthetic(b.Trace, 6000, 42)
		for i := 0; i < b.Count; i++ {
			if e := r.Context().Err(); e != nil {
				return
			}
			event := events[(a.driveIndex+i)%len(events)]
			_, e := a.Cache.Get(r.Context(), event.Key)
			if e != nil {
				fail(502, e)
				return
			}
		}
		a.driveIndex = (a.driveIndex + b.Count) % 6000
		write(w, 200, a.Cache.Snapshot())
	case "/api/replay":
		var b struct {
			Trace string `json:"trace"`
			Count int    `json:"count"`
		}
		if e := body(w, r, &b); e != nil {
			fail(400, e)
			return
		}
		if b.Count < 512 || b.Count > 6000 {
			fail(400, errors.New("count must be 512..6000"))
			return
		}
		var events []replay.Event
		if b.Trace == "web12" {
			v, e := replay.Public("data/web12.bin.gz", b.Count)
			if e != nil {
				fail(500, e)
				return
			}
			events = v
		} else if b.Trace == "hotset" || b.Trace == "scan" || b.Trace == "shift" {
			events = replay.Synthetic(b.Trace, b.Count, 42)
		} else {
			fail(400, errors.New("unknown trace"))
			return
		}
		rows := []replay.Row{}
		for _, mode := range []string{"fixed-ttl", "plain-lru", "lfu", "tinylfu", "adaptive"} {
			rows = append(rows, replay.Run(b.Trace, mode, events))
		}
		write(w, 200, map[string]any{"rows": rows, "scope": "deterministic modeled origin; 10ms synthetic clock; no network; early refresh inline"})
	case "/api/evidence":
		if r.Method != "GET" {
			fail(405, errors.New("GET required"))
			return
		}
		p, e := os.ReadFile(filepath.Join("docs/results", "campaign.json"))
		if e != nil {
			write(w, 200, map[string]any{"rows": []any{}, "notes": "not measured"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(p)
	default:
		fail(404, errors.New("not found"))
	}
}
