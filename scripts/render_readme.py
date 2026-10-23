"""Render measured claims from saved campaign JSON; no imported benchmark numbers."""

import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
report = json.loads((root / "docs/results/campaign.json").read_text())


def row(trace: str, mode: str) -> dict:
    return next(r for r in report["rows"] if r["trace"] == trace and r["mode"] == mode)


s, b = row("scan", "adaptive"), row("scan", "plain-lru")
p, pb = row("web12", "adaptive"), row("web12", "plain-lru")
text = f"""# Tidal

Tidal tests cache policies against observed traffic while accounting for origin work and stale responses.

## Result

Scan-heavy synthetic replay: **{100 * b["hit_rate"]:.2f}% → {100 * s["hit_rate"]:.2f}% hit rate**, {b["origin_calls"]:,} → {s["origin_calls"]:,} origin calls over {s["requests"]:,} requests, including {s["refreshes"]:,} adaptive early refreshes.  
Complete public Web12 key order: adaptive **{100 * p["hit_rate"]:.2f}% vs LRU {100 * pb["hit_rate"]:.2f}% hits**, {p["origin_calls"]:,} vs {pb["origin_calls"]:,} origin calls over {p["requests"]:,} requests — adaptation hurts here.  
Web12 with synthetic freshness annotations: **{p["stale_serves"]:,} adaptive vs {pb["stale_serves"]:,} LRU stale serves**; timing, writes, TTL and invalidations are modeled, not original public metadata. [Full comparison and raw evidence](docs/results/COMPARISON.md).

## Architecture

```mermaid
flowchart LR
  UI[React color-band lab] --> API[Authenticated Go API]
  API --> Active[Active cache / coalesced origin fetch]
  Active --> Origin[Actual local synthetic HTTP origin]
  Active --> Policies[LRU / LFU / TinyLFU admission]
  API --> Shadows[Bounded key-only shadow caches]
  Shadows --> Selection[Window scores / hysteresis / cooldown]
  Selection --> Policies
  Traffic[Seeded and public key replay] --> Core[Same cache core / deterministic clock]
  Core --> Evidence[Hits / origin work / stale versions]
```

## Run it

Go, Node/npm and Python with venv are prerequisites. Pinned package/toolchain downloads happen during setup; the licensed public trace is included.

```sh
make demo
# Open http://127.0.0.1:8095; replay traffic or query the live cache. Ctrl-C stops it.
make lint test bench
```

[Exact demo/recording steps](docs/DEMO.md), [quality report](docs/QUALITY.md), [design](docs/DESIGN.md), [interview kit](INTERVIEW.md), [follow-up drill](DRILL.md).

## Decisions and limits

- Original Go policy/refresh/invalidation core; TinyLFU-style sketch admission over an LRU victim, not W-TinyLFU. A global generation fences in-flight invalidation; a real failure test retains the old-response rejection. [Race/reference/HTTP output](docs/results/tests.log), [ADRs](docs/adr/).
- The three synthetic traces and complete public trace use equal resident object capacities. Shadow/sketch metadata is extra memory. Public Web12 supplies key order only, with [upstream attribution and hash](docs/results/trace-provenance.json). Object sizes, request clock and content versions are synthetic. This is not a public production freshness result.
- Early refreshes count as origin work. Adaptation loses on several traces, including stable/shifted hot sets and Web12 origin load; the table retains those results. More hits do not guarantee fresher responses or lower total cost.
- Replay executes refresh inline against a modeled origin; Live cache uses bounded asynchronous refresh and an actual loopback HTTP origin. Replay is not service throughput. HTTP load capacity, tail latency under load and production dollar savings are **not measured**.
- The API caches typed synthetic JSON objects, not arbitrary HTTP responses. Unannounced origin changes can stay stale within the TTL contract; no invalidation oracle or stale-on-error fallback is promised. [Caught initial null-list UI failure](docs/results/caught-initial-null-windows.json) and corrected HTTP regression test are retained.
- Local bearer-authenticated demo, disposable cache/origin state; remote TLS terminates at an external proxy. Docker/Render are prepared only: **not deployed**. Cinder integration is [proposed separately](docs/CINDER_INTEGRATION_PROPOSAL.md); existing projects remain unchanged. No company adoption is claimed.

## Next

Isolated tuning studies for TTL versus eviction versus refresh; byte/cost-aware admission; faster LFU/expiry structures; replay with original timestamp/size/update metadata when legally available; cooperative replay cancellation, stronger operational quotas, production origin contracts and a separately tested Cinder adapter. [Migration record](docs/migration/tidal.md).
"""
(root / "README.md").write_text(text)
lines = [
    "# Tidal measured comparison",
    "",
    report["notes"],
    "",
    f"Go {report['go']}, {report['os']}/{report['arch']}; seed {report['seed']}. Elapsed replay time is retained in raw JSON, not claimed as HTTP performance.",
    "",
    "| Trace | Mode | Requests | Hit rate | Origin calls | Refreshes | Stale serves | Stale rate | Switches |",
    "|---|---|---:|---:|---:|---:|---:|---:|---:|",
]
for r in report["rows"]:
    lines.append(
        f"| {r['trace']} | {r['mode']} | {r['requests']:,} | {100 * r['hit_rate']:.2f}% | {r['origin_calls']:,} | {r['refreshes']:,} | {r['stale_serves']:,} | {100 * r['stale_rate']:.3f}% | {r['switches']} |"
    )
lines += [
    "",
    "[Raw aggregate/window JSON](campaign.json), [raw run stdout](bench-run.log), [repeated hit-path microbenchmark](core-bench.log), [public source/license/hash](trace-provenance.json).",
    "",
    "## What the results say",
    "",
    "The selector changes policy on scan-heavy traffic, but its advantage in total origin calls is small once early refreshes are counted. On stable and shifting hot sets the short traffic-based TTL and early refresh add substantial origin work. On Web12, adaptive slightly reduces stale observations under the synthetic update model while increasing origin work and lowering hit rate. No general improvement is established. Every losing mode remains visible.",
    "",
    "The active eviction rule follows independently warmed fixed-TTL shadows, while adaptive TTL/refresh and active resident history differ. These shadows are advisory. An isolated study is needed before attributing improvements to a single feature. The public trace contains no original freshness information; stale counts apply only to Tidal's synthetic annotations. All result rows are cold-start runs of one deterministic seed, not a confidence interval or a production workload guarantee.",
    "",
    "The Go hit-path microbenchmark includes locking, observation and shadow bookkeeping. It is not an HTTP request benchmark. Printed allocation counts are rounded per operation: a rounded zero allocations/op with nonzero bytes/op must not be presented as allocation-free.",
]
(root / "docs/results/COMPARISON.md").write_text("\n".join(lines) + "\n")
print("Generated README.md and measured comparison from saved campaign.json")
