# Tidal quality report

Scope: local Go cache with typed synthetic HTTP origin, owned policies/refresh/invalidation, deterministic public/synthetic replay, and original React lab. Deployment and Cinder integration are prepared/proposed only.

| Gate | Evidence | Status |
|---|---|---|
| Owned hard core and explanation | internal/cache, DESIGN.md and ADRs | PASS |
| One-command fresh clone | docs/results/fresh-clone/verification.json records actual make demo | Verification recorded when complete |
| UI shows live behavior | browser/review.json, actual HTTP receipts, screenshots and recording | PASS locally |
| Unit/integration/reference/concurrency | race tests, independent LRU model, invalidation race, real HTTP origin | PASS |
| Benchmarks saved | campaign.json, bench-run.log, core-bench.log, complete public trace provenance | PASS |
| Demonstrated failure | withheld-invalidation negative control, rejected old fill, outage and caught null-array UI regression | PASS |
| ADRs | shadows/TTL, replay/live distinction, color streams | PASS |
| Interview kit | INTERVIEW.md and DRILL.md | PASS |
| Dependency audit | dependency-scan.log, Go reachability plus Python/npm scans | PASS locally |
| CI green | exact final head verified by coordinator in TIDAL_CI_VERIFICATION.json | Pending final verification |
| No unmeasured claims | README generated from campaign JSON; negative outcomes retained | PASS within explicit measurement scope |

Remaining scope: no hosted service, generic HTTP proxy, original public freshness metadata, byte-based capacity control, isolated tuning effectiveness study, production tail-latency/load measurement, distributed consistency, tenant fairness or Cinder integration. Static origin max-age is a reuse cap, not an invalidation oracle. Adaptive mode loses on multiple traces; no general benefit is claimed.

The empty-window bug is demonstrated by caught-initial-null-windows.json and caught-browser-null-window.log, fixed at the API boundary, then guarded by an HTTP assertion and browser rerun. A formatting failure is retained separately; it is not described as a runtime bug. Docker execution is verified on Linux CI rather than claimed from a native local run.
