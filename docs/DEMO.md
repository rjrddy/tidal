# Tidal demo and recording

Prerequisites: Go with toolchain downloading enabled, Node/npm and Python with venv. `make setup` downloads the pinned Go toolchain when needed and installs isolated Python/npm packages. The licensed public Web12 fixture is included and hash-checked; demo startup needs no trace account or external data service.

```
make demo
```

Open http://127.0.0.1:8095 . The loopback demo token is `local-demo`. Replay lab runs a bounded on-demand deterministic comparison; Recorded runs reads saved benchmark evidence. Live cache uses an actual loopback HTTP origin and the wall-clock cache. These are deliberately distinct views.

In Live cache: fetch a new key, fetch it again, withhold an invalidation and change the source version, then fetch before expiry to observe a stale response. Deliver an invalidation, change the source and fetch again. Inject an outage and request a missing key: it must fail, then recover the origin and retry. Drive traffic to produce completed shadow-scoring windows. A real public-data key sequence does not imply real origin freshness metadata.

Set `TIDAL_ADDR` or `TIDAL_TOKEN` via environment as needed. Only loopback accepts the public default token. External binding requires a private token and HTTPS termination at a proxy. Ctrl-C stops the server and its owned synthetic origin. Cache/origin state is disposable; there is no persistence guarantee.

## Ninety-second recording plan

- 0–15s: run the scan replay; inspect hit rate and origin/refresh/stale counts for all policies.
- 15–30s: open the complete public Web12 evidence. Show the losing adaptive result and the synthetic-clock/freshness caveat.
- 30–45s: fetch the same synthetic key twice over the live HTTP path; inspect miss/hit and source version.
- 45–60s: withhold invalidation, mutate source, expose staleness, then deliver invalidation and verify a fresh fetch.
- 60–75s: inject origin failure; a missing/expired key fails; recover and retry.
- 75–90s: drive traffic, inspect actual shadow window rates/selected rule, narrow the viewport and fetch another key.

Automated recorder: install `requirements-browser.txt` into the local venv and run `.venv/bin/python -m playwright install chromium ffmpeg`, then `.venv/bin/python scripts/review.py --record`. It owns/stops its server and records changed real requests until the required duration; no static padding. Verify actual duration with `ffprobe docs/results/browser/tidal-demo.webm`. A matching installed browser cache can be selected with `PLAYWRIGHT_BROWSERS_PATH`.

## Reproduce evidence

`make lint test bench` runs the requested gates, race tests, complete public + synthetic replay and repeated hit-path microbenchmarks. `make scan` audits Go reachability and Python/npm dependencies. Raw outputs remain under docs/results/; `scripts/render_readme.py` generates the measured README and campaign table. Benchmark timings exclude HTTP unless explicitly labeled live.

## Hosting preparation only

Dockerfile and Render free-plan config are prepared. No account/service/deployment was created. Render's free services have ephemeral storage and other operational limits: https://render.com/docs/free . Set a private `TIDAL_TOKEN`, use HTTPS at the edge and enter the token in connection settings. The origin remains synthetic; this is not a production origin connection. Cinder integration remains a separate proposal with no existing-project changes.
