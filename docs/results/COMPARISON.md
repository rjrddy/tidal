# Tidal measured comparison

Equal 300-object capacities; 10ms synthetic clock/request; versions mutate every 73 requests with deterministic dropped invalidations. Public Web12 keys only; timestamps/versions/TTLs synthetic. Early refresh inline in replay, asynchronous in HTTP. Origin calls include refresh. Not service throughput; no object-byte sizing.

Go go1.27.3, darwin/arm64; seed 42. Elapsed replay time is retained in raw JSON, not claimed as HTTP performance.

| Trace | Mode | Requests | Hit rate | Origin calls | Refreshes | Stale serves | Stale rate | Switches |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| hotset | fixed-ttl | 30,000 | 74.06% | 7,782 | 0 | 215 | 0.717% | 0 |
| hotset | plain-lru | 30,000 | 87.44% | 3,767 | 0 | 916 | 3.053% | 0 |
| hotset | lfu | 30,000 | 87.45% | 3,766 | 0 | 916 | 3.053% | 0 |
| hotset | tinylfu | 30,000 | 87.45% | 3,766 | 0 | 916 | 3.053% | 0 |
| hotset | adaptive | 30,000 | 82.81% | 8,152 | 2,996 | 288 | 0.960% | 0 |
| scan | fixed-ttl | 30,000 | 20.49% | 23,852 | 0 | 41 | 0.137% | 0 |
| scan | plain-lru | 30,000 | 24.29% | 22,713 | 0 | 59 | 0.197% | 0 |
| scan | lfu | 30,000 | 27.45% | 21,764 | 0 | 107 | 0.357% | 0 |
| scan | tinylfu | 30,000 | 27.80% | 21,661 | 0 | 131 | 0.437% | 0 |
| scan | adaptive | 30,000 | 27.11% | 22,641 | 774 | 70 | 0.233% | 1 |
| shift | fixed-ttl | 30,000 | 78.83% | 6,352 | 0 | 536 | 1.787% | 0 |
| shift | plain-lru | 30,000 | 90.68% | 2,795 | 0 | 1,826 | 6.087% | 0 |
| shift | lfu | 30,000 | 90.65% | 2,806 | 0 | 1,827 | 6.090% | 0 |
| shift | tinylfu | 30,000 | 89.88% | 3,037 | 0 | 1,795 | 5.983% | 0 |
| shift | adaptive | 30,000 | 86.28% | 6,957 | 2,842 | 385 | 1.283% | 0 |
| web12 | fixed-ttl | 95,607 | 44.28% | 53,274 | 0 | 290 | 0.303% | 0 |
| web12 | plain-lru | 95,607 | 48.53% | 49,209 | 0 | 499 | 0.522% | 0 |
| web12 | lfu | 95,607 | 43.56% | 53,957 | 0 | 560 | 0.586% | 0 |
| web12 | tinylfu | 95,607 | 41.53% | 55,901 | 0 | 573 | 0.599% | 0 |
| web12 | adaptive | 95,607 | 48.12% | 50,329 | 731 | 404 | 0.423% | 0 |

[Raw aggregate/window JSON](campaign.json), [raw run stdout](bench-run.log), [repeated hit-path microbenchmark](core-bench.log), [public source/license/hash](trace-provenance.json).

## What the results say

The selector changes policy on scan-heavy traffic, but its advantage in total origin calls is small once early refreshes are counted. On stable and shifting hot sets the short traffic-based TTL and early refresh add substantial origin work. On Web12, adaptive slightly reduces stale observations under the synthetic update model while increasing origin work and lowering hit rate. No general improvement is established. Every losing mode remains visible.

The active eviction rule follows independently warmed fixed-TTL shadows, while adaptive TTL/refresh and active resident history differ. These shadows are advisory. An isolated study is needed before attributing improvements to a single feature. The public trace contains no original freshness information; stale counts apply only to Tidal's synthetic annotations. All result rows are cold-start runs of one deterministic seed, not a confidence interval or a production workload guarantee.

The Go hit-path microbenchmark includes locking, observation and shadow bookkeeping. It is not an HTTP request benchmark. Printed allocation counts are rounded per operation: a rounded zero allocations/op with nonzero bytes/op must not be presented as allocation-free.
