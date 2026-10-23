# Tidal interview kit

## Two-minute pitch

Tidal is a cache whose hard part is deciding when changing policy helps rather than merely moving a metric. I wrote LRU, resident LFU, TinyLFU-style admission, key-only shadow scoring, hysteretic switching, traffic-aware TTL and bounded early refresh. A mutex protects metadata; origin I/O happens outside it, with same-key coalescing and invalidation fencing. The live Go service really calls a local HTTP origin. A distinct replay path uses the same core with a deterministic clock and version model.

The useful story is that adaptation does not always win. The complete raw table includes the stable hot set, scans, shifted demand and public Web12 order. Early refresh counts toward origin load; unannounced source updates expose stale responses. The public file has no original freshness metadata, so its update/TTL timing is explicitly synthetic. I can show a caught invalidation race, an origin outage, and the fresh-start null-array UI bug that the HTTP/browser checks caught. The project demonstrates a measurable experimental system rather than a production cache-efficiency claim.

## Ten-minute walkthrough

Start with `policy.go`: map/list residents, recency updates, exact resident LFU victim scan, count-min sketch estimates, aging and admission ties. Explain why this is TinyLFU-style admission over LRU rather than W-TinyLFU. Show the expiration sweep and the cost of linear scans.

Then follow `Manager.Get`: metadata lock, shadow observation, current cache lookup, origin flight, context-aware waiter and a recheck that prevents duplicate loads after another fill. Walk through global invalidation epoch fencing; the concurrency test holds an old response, invalidates, releases it and requires retry. Explain the conservative discarded-unrelated-load tradeoff.

Follow the selector's completed windows, margin, challenger streak and cooldown. Contrast independently warmed fixed-TTL shadows with the active cache's real payload/TTL history. A shadow is an advisor, not a source of values or a guarantee. Then explain the EMA reuse horizon, authoritative max-age cap and bounded asynchronous refresh.

Open the UI. Show deterministic replay, saved complete evidence and real HTTP behavior as separate views. Withhold invalidation to expose an old response; deliver one to force a new fetch. Break the synthetic origin and request a missing key; it must fail. Explain why the stale oracle exists only in this controlled fixture.

Finish with `campaign.json`, COMPARISON.md, the public license/hash, race/reference tests, browser receipts and actual video. Explain losing cases before proposing improvements. The current integration is in-process; Cinder is only a separate proposal. Hosting configuration is prepared, no service deployed.

## Five tradeoffs

- One mutex is easier to reason about than sharded mutable policy structures, but serializes metadata work and limits throughput.
- Key-only shadows avoid origin amplification, but their independent residency/fixed TTL may predict the active cache poorly.
- A global generation prevents unbounded invalidation tombstones, but can discard unrelated in-flight loads.
- Simple EMA TTL and early refresh are explainable, but can add origin work and lower hits; losing results remain visible.
- Exact resident LFU and eager expiration sweeps are small and inspectable, but linear in capacity and unsuitable for a large unmodified deployment.

## Five failure modes

- Invalidation crosses an origin request: reject the old fill and boundedly retry; never publish it as a newly valid entry.
- Origin outage: fresh hits remain usable; expired/missing entries fail. No stale-on-error fallback is silently enabled.
- Unannounced update: a response may remain stale within its reuse period. Refresh/expiry bound reuse, not knowledge of writes.
- Policy misprediction: margin/streak/cooldown damp oscillation, but do not guarantee improvement. The full comparison exposes the losses.
- Empty initial metrics break the UI: the browser caught a null-window-list contract mismatch; an actual HTTP regression test now requires an array.

## Fifteen likely questions

1. What is being optimized? A recent shadow hit signal advises eviction. Origin load and staleness are measured, not jointly optimized by a hidden cost function.
2. Why does the adaptive mode sometimes lose? Shorter TTLs and early refresh cost origin calls; shadow residency differs from active state; the future workload can change.
3. Does TinyLFU evict the least-frequent item? Not here. It admits a candidate only if its estimated recent frequency beats the LRU victim. Resident LFU is a separate mode.
4. What does the sketch guarantee? Counts are bounded, collision-prone estimates with periodic aging. No exact-frequency guarantee follows.
5. Why no doorkeeper/window segment? This deliberately small baseline omits them and is labeled accordingly; do not claim full W-TinyLFU behavior.
6. Can a policy change fabricate cached values? No. Selection changes active eviction/admission rules only; active payloads are retained and shadow payloads do not exist.
7. Is the TTL allowed to exceed origin max-age? No. The final TTL is capped by the origin's response contract, including an uncacheable zero-age response.
8. Does max-age mean no stale versions? No. A write can occur within the advertised reuse period; only a delivered invalidation conveys that knowledge.
9. How are stampedes handled? Same-key overlapping misses join a flight; a locked recheck avoids a just-completed fill being missed. Overall flights and refresh jobs are bounded.
10. Can a canceled waiter cancel everyone? It stops its own wait. A load uses the initiating request context; leader cancellation can fail the shared attempt. A retry by a later request is separate.
11. Why use a global invalidation epoch? It prevents late resurrection without retaining unbounded per-key generations. It can sacrifice unrelated work conservatively.
12. Is replay identical to production? Policy code is shared, but the replay clock/model and inline refresh differ from live wall-clock HTTP/async refresh. Claims name that scope.
13. What is public in Web12? Anonymous key-access order. Original timestamps, sizes, payloads, source updates and freshness metadata are not supplied.
14. Is a rounded zero allocations/op allocation-free? No. The raw microbenchmark reports bytes too; amortized occasional window allocations can round to zero per operation.
15. What does the race detector establish? No detected data races on the tested policy, flight, invalidation and real HTTP paths. It is finite evidence, not a proof of all interleavings.

## Three more months

Isolate eviction, TTL and refresh effects with broader trace sweeps; obtain legally usable original timing/size/update metadata; measure byte budgets including shadows and allocator costs; improve LFU/expiry complexity; benchmark real service tail latency under bounded load; add cancellation to replay and stricter origin contracts; independently test a Cinder adapter and an authenticated deployment. Do not promote a tuning rule until the retained losses are understood.
