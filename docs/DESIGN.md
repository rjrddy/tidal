# Tidal: choosing what to keep without an oracle

A policy that increases hit rate can still add origin traffic through refresh, serve older versions, or spend too much CPU/memory watching alternatives. Tidal keeps all those costs visible. Its objective is inspectable online decisions, not a claim that adaptation always wins.

## Actual core and invariants

`internal/cache/policy.go` owns a map plus recency list. LRU chooses the tail; exact resident LFU chooses minimum frequency with sequence tie-breaks and scans resident entries. LFU does not age resident frequency. TinyLFU-style admission compares a candidate's recent count-min estimate against the LRU victim, rejecting ties. Its bounded counter sketch ages periodically. This is a small frequency-admission baseline inspired by the original paper, not W-TinyLFU or a reproduction of all paper algorithms: https://arxiv.org/abs/1512.00727 . Expired records are removed before admission comparison.

`Manager` serializes metadata mutations under one mutex, releases it for origin I/O, and coalesces concurrent same-key misses. A second check under the flight lock prevents a just-completed fill from causing another redundant request. Values contain immutable strings. TTL expiry uses the supplied clock, including the exact expiry boundary. The authoritative origin max-age caps every TTL; no stale-on-error fallback exists after expiry.

Explicit invalidation increments a global epoch and removes the key from active/shadow stores. An older in-flight fill is discarded and its waiters retry, bounded by the retry budget. Global fencing can discard unrelated loads too; this intentionally avoids an unbounded per-key generation map. A cache hit that linearizes before an invalidation can still be returned. Unannounced writes are fundamentally unknowable until another fetch, refresh or expiry.

The observation map, cache residents, flight table, recent window history and sketches are bounded. Active object capacity is not a total memory budget: independent shadows and sketch/list metadata cost extra memory. Sketch collisions can overestimate frequency. LFU eviction and expiration sweep are linear in resident capacity; the code prioritizes clarity over large-cache scalability.

## What the shadows mean

Each request updates key-only LRU, LFU and TinyLFU shadows with a fixed configured TTL. They never call the origin. Their window hit counts advise eviction selection. A challenger needs an absolute margin, consecutive wins and a cooldown. It changes only the active victim/admission rule, retaining actual resident payloads. No payload is fabricated from shadow contents. Active recency/frequency history differs from a shadow's history, and adaptive TTL differs from the shadows' fixed TTL: shadow advantage is advisory, not a guaranteed counterfactual hit rate for the real active cache.

A simple EMA of inter-arrival time sets an adaptive reuse horizon, clamped and capped by origin max-age. A frequently requested resident approaching expiry may refresh early. Async refresh slots and origin flights are bounded. Refresh rechecks current expiry to avoid redundant queued refreshes after another fill. Errors preserve only a still-fresh entry; expiry still fails closed. The response labels refresh eligibility; the metric counts actual refresh attempts.

## Live HTTP boundary

`internal/service` starts a real loopback synthetic HTTP origin and uses an HTTP client with context/deadline and response-size limits. This is a typed JSON fixture-origin cache, not a general RFC-compliant web proxy; arbitrary origins and SSRF targets are not accepted. Live requests are real wall-clock cache/HTTP calls. The driver sends bounded seeded requests against that path. The origin is deliberately synthetic and disposable; restarts lose its versions and cache state.

The API checks bearer auth, strict bounded JSON, methods, key shapes and work counts. The public local token is accepted only on a loopback listener; external binding requires a private token and a TLS proxy. There is bounded request/flight/refresh admission, but no tenant fairness or production identity system. Structured logs omit tokens, values and request query strings. State exposes process-local hit/origin/refresh/error/invalidation counters and completed policy windows.

The UI's stale-observed field compares a response against the in-process synthetic origin's current version. This oracle is test instrumentation, not a feature a real arbitrary cache could possess. With concurrent origin changes it is a comparison at observation time, not a claim that the response violated its linearization point.

## Replay protocol

The replay uses the same Manager and store code, but a deterministic clock and origin-version model. Refresh is inline after capturing the served value, unlike asynchronous live HTTP. No network is involved; elapsed replay CPU time is not service throughput. All modes have equal object capacities, not equal memory. Fixed-TTL LRU, plain LRU, LFU, TinyLFU and adaptive variants expose refresh/origin/stale counts separately. Cold start is included. A sample point covers a complete request window; trailing incomplete windows remain in aggregate results.

Three synthetic workloads cover a stable hot set, one-hit scans mixed with hot keys, and phase-changing demand. All are seeded. The complete Web12 trace supplies public anonymous key order only. Its original timestamps, object sizes, values and freshness state are not present. Tidal adds a constant synthetic request interval, equal objects, version writes, TTL and intentionally dropped invalidations. These annotations are not claims about the source system's freshness. See `docs/results/trace-provenance.json`, the upstream pinned path, and `data/LICENSE-public-trace.txt`.

Metrics: hit rate is hits / offered requests. Origin load counts misses plus actual early refresh attempts in the sequential replay; the accounting identity is tested there. Live concurrency, coalescing, errors and retries can change that identity. Stale rate is responses whose version differs from synthetic origin truth / offered requests, including unannounced changes. More hits alone do not establish a better outcome.

## Sources and visual research

- TinyLFU original research: https://arxiv.org/abs/1512.00727
- Public trace and original reader/provenance: https://github.com/cache2k/cache2k-benchmark/tree/738948d580e757377e755f0c3bfa2ac8f818ffcd
- Go standard library HTTP/context/runtime documentation: https://pkg.go.dev/net/http and https://go.dev/doc/go1.27
- Color interaction research: https://www.moma.org/artists/97-josef-albers and https://www.moma.org/momaorg/shared/pdfs/docs/learn/mondays/Josef-Albers-Color-combinations.pdf . Original CSS/SVG; no copied artwork.
