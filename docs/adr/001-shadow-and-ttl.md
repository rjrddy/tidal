# Shadows advise; they never fetch

Each request updates bounded key/expiry-only LRU, LFU and TinyLFU shadows. Fixed comparison TTL makes their hit rates an eviction signal, not an oracle for the active adaptive TTL. At each window, the best challenger needs a strict absolute hit-rate margin and consecutive wins with cooldown before changing the active victim rule. Active resident entries are retained: no imaginary shadow payload is promoted. Existing frequencies and recency remain local to active history, so shadow advantage is advisory rather than a promised improvement.

Traffic-aware TTL uses an inter-arrival EMA multiplied by a small reuse horizon, clamped to configured limits and always capped by origin max-age. Frequently observed entries nearing expiry can refresh early. This can add origin load or hurt hit rate. Replay includes those costs and does not silently disable losing settings.

Explicit invalidation fences an in-flight response using a global generation. Any invalidation can discard an unrelated in-flight load as a conservative tradeoff; no unbounded per-key tombstones. Get retries a bounded number of times. Unannounced writes can still be stale until expiry/refresh: max-age is a bounded reuse contract, not an invalidation feed.
