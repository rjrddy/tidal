# Tidal follow-up drill

1. Where is the cache's linearization point? Metadata lookup/fill/invalidation occurs under the Manager mutex. A returned hit can precede a concurrent later invalidation.
2. Can an old response fill after invalidation? An epoch mismatch rejects it and wakes waiters with a retryable error; retries are bounded.
3. What does a global epoch over-invalidate? Every older in-flight load, even for another key. That avoids tombstone growth but wastes some work.
4. Why is the cache allowed to return an old version? Unannounced changes are unknown, and a fresh TTL entry remains reusable by contract. The synthetic oracle exposes the tradeoff.
5. Does early refresh guarantee a fresh serve? No. The triggering hit can return its old value while refresh runs; its version comparison may be stale.
6. How are async refreshes bounded? A fixed refresh-slot channel plus bounded flight map and same-key coalescing; load rechecks whether another refresh already completed.
7. Does refresh eligibility mean an origin call occurred? No. The response labels eligibility; only the actual attempt increments refresh/origin counters.
8. Can TinyLFU admit a one-hit scan over a hot victim? Its estimate normally rejects it, but sketch collisions can inflate a candidate. It is approximate.
9. What happens on equal sketch counts? Candidate is rejected. That resists churn but can delay useful newcomers.
10. Does LFU age? Resident frequency does not; the sketch does. Old popular LFU residents can resist a shifted hot set.
11. What costs are hidden by equal capacities? Shadow residents, sketches, lists and policy metadata. Equal object count is not equal total memory or bytes stored.
12. Why are shadow scores not exact active counterfactuals? Fixed TTL, independent warmed residency and past rule history differ from the active cache's TTL/refresh/history.
13. Can the selector see future requests? No, only observed current/past key accesses and completed scores. It is not a future-aware oracle.
14. Why a challenger streak and cooldown? To reduce reacting to noisy short windows. They can also delay a beneficial switch.
15. What is counted as origin load? Every actual loader attempt including refresh. Sequential replay also verifies misses plus refresh accounting; live coalescing/errors/retries complicate that identity.
16. What is the public freshness result? There is none. Public key order is replayed with synthetic time/version/invalidation annotations; stale counts apply only to that model.
17. Why no cache persistence? Values and synthetic origin versions are disposable; a restart reconstructs them. Persistence would require a separate freshness/reconciliation contract.
18. Why not call it an arbitrary HTTP reverse proxy? It caches typed JSON objects from a fixed local origin, not full HTTP caching semantics or arbitrary destinations.
19. What actual failure did browser QA catch? An empty initial Go slice serialized as null, crashing the live-window view. The API now returns an empty array and a real HTTP regression assertion protects it.
20. What would justify integrating Cinder? Both independent gates plus RESP-based expiry/eviction/invalidation/outage tests and measurements of the extra hop and combined memory; standalone benchmark numbers are not integration evidence.
