package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRealHTTPOriginAndCache(t *testing.T) {
	a, e := New("test-token", "../../apps/web/dist")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	s := httptest.NewServer(a)
	defer s.Close()
	req := func(path, body, token string) (int, map[string]any) {
		t.Helper()
		method := "GET"
		if body != "" {
			method = "POST"
		}
		r, _ := http.NewRequest(method, s.URL+path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var v map[string]any
		if e := json.NewDecoder(response.Body).Decode(&v); e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, v
	}
	if code, v := req("/api/state", "", "test-token"); code != 200 || v["cache"].(map[string]any)["windows"] == nil {
		t.Fatal("state arrays must be arrays, including fresh-start windows")
	}
	if code, _ := req("/api/state", "", "wrong"); code != 401 {
		t.Fatal("auth")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := req("/api/get?key=k1", "", "test-token")
			if code != 200 {
				t.Errorf("HTTP %d", code)
			}
		}()
	}
	wg.Wait()
	if n := a.Origin.calls.Load(); n != 1 {
		t.Fatalf("coalescing origin calls=%d", n)
	}
	_, _ = req("/api/mutate", `{"key":"k1","invalidate":false}`, "test-token")
	code, v := req("/api/get?key=k1", "", "test-token")
	if code != 200 || v["stale_observed"] != true {
		t.Fatal("negative control stale not observed", v)
	}
	_, _ = req("/api/mutate", `{"key":"k1","invalidate":true}`, "test-token")
	_, v = req("/api/get?key=k1", "", "test-token")
	if v["stale_observed"] != false {
		t.Fatal("invalidation failed")
	}
	_, _ = req("/api/fault", `{"fail":true}`, "test-token")
	time.Sleep(1300 * time.Millisecond)
	if code, _ := req("/api/get?key=k1", "", "test-token"); code != 502 {
		t.Fatal("expired response served on failure")
	}
	_, _ = req("/api/fault", `{"fail":false}`, "test-token")
	if code, _ := req("/api/get?key=k1", "", "test-token"); code != 200 {
		t.Fatal("recovery")
	}
	for _, body := range []string{`{"trace":"madeup","count":512}`, `{"trace":"hotset","count":999999}`, `{"count":true}`, `{"trace":"hotset","count":512,"unknown":1}`, `{} {}`} {
		if code, _ := req("/api/replay", body, "test-token"); code != 400 {
			t.Fatal("bad input accepted", body)
		}
	}
	t.Log("real HTTP origin: coalescing, stale negative control, invalidation, failure, TTL expiry, recovery, input/auth gates pass")
}
func TestFailedOriginContract(t *testing.T) {
	a, e := New("test-token", ".")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	r, e := a.client.Get(a.originURL + "/?key=invalid")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 400 {
		b, _ := io.ReadAll(r.Body)
		t.Fatal(fmt.Sprint(r.StatusCode, string(b)))
	}
}
