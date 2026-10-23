# Share policy code, distinguish clocks

Replay calls the same Manager and store code as HTTP. It uses a deterministic clock and origin-version model; early refresh completes inline after capturing the served value. Live HTTP uses wall time and bounded asynchronous refresh over a real loopback HTTP origin. Those are different execution environments. Replay hit rates and origin counts are not HTTP throughput measurements.

Public Web12 provides anonymous integer key order only. Its entire compressed source is shipped with upstream CC BY attribution and a pinned hash. The imposed clock, equal object sizes, TTLs, writes and dropped invalidations are synthetic annotations; no real public-trace freshness claim follows.

No Cinder integration: Tidal first passes its own checks. A separate proposal can compare a Cinder-backed path after both are independently verified, without implying the current cache already uses Cinder.
