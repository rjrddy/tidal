# Cinder integration proposal — not implemented

Both services need independent green gates before connecting them. Current Tidal owns in-process value storage and policy metadata; it does not talk to Cinder. Cinder and all existing projects remain unchanged.

A small optional adapter can later place opaque synthetic payloads in Cinder using its supported RESP operations while Tidal retains eviction/expiry decisions. It must propagate explicit invalidation, delete evicted keys, distinguish remote storage errors from misses, bound local/remote divergence and never resurrect a fenced response. Cinder's own TTL must not be used to extend the origin max-age.

Before merging an integration: rerun both independent gates; add actual RESP socket tests for expiry, deletion, outages, restart and invalidation races; measure the extra hop and account for memory in both processes. A disconnected Cinder must not cause a stale fallback. Treat remote snapshots separately from disposable cache state. No result from either standalone project is an integration benchmark.
