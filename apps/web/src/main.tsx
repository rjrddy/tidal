import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
type Window = {
  at: number;
  selected: string;
  rates: Record<string, number>;
  switched: boolean;
};
type Stats = {
  requests: number;
  hits: number;
  origin_calls: number;
  refreshes: number;
  errors: number;
  invalidated_loads: number;
  switches: number;
  selected: string;
  resident: number;
  inflight: number;
  windows: Window[];
};
type State = {
  cache: Stats;
  origin_calls: number;
  origin_failing: boolean;
  stale_observed: number;
  keys: string[];
  capacity: number;
  scope: string;
};
type Sample = {
  at: number;
  hits: number;
  origin: number;
  stale: number;
  policy: string;
};
type Row = {
  trace: string;
  mode: string;
  requests: number;
  hits: number;
  hit_rate: number;
  origin_calls: number;
  origin_per_request: number;
  refreshes: number;
  stale_serves: number;
  stale_rate: number;
  switches: number;
  samples: Sample[];
  windows: Window[];
};
type Get = {
  result: {
    value: { key: string; version: number; body: string };
    hit: boolean;
    refresh_eligible: boolean;
    policy: string;
  };
  stale_observed: boolean;
  elapsed_ms: number;
};
const modes = ["fixed-ttl", "plain-lru", "lfu", "tinylfu", "adaptive"];
const names: Record<string, string> = {
  "fixed-ttl": "Fixed TTL",
  "plain-lru": "Plain LRU",
  lfu: "LFU",
  tinylfu: "TinyLFU",
  adaptive: "Tidal adaptive",
};
const pct = (n: number) => `${(100 * n).toFixed(1)}%`;
const fmt = (n: number) => n.toLocaleString();
function App() {
  const [tab, setTab] = useState("replay");
  const [token, setToken] = useState("local-demo");
  const [state, setState] = useState<State | null>(null);
  const [trace, setTrace] = useState("scan");
  const [rows, setRows] = useState<Row[]>([]);
  const [recorded, setRecorded] = useState<Row[]>([]);
  const [scope, setScope] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [key, setKey] = useState("k17");
  const [result, setResult] = useState<Get | null>(null);
  const [announce, setAnnounce] = useState(true);
  const [receipt, setReceipt] = useState("");
  const mounted = useRef(true);
  async function api<T>(path: string, body?: unknown): Promise<T> {
    const response = await fetch(`/api/${path}`, {
      method: body === undefined ? "GET" : "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const value = await response.json();
    if (!response.ok) throw new Error(value.error || `HTTP ${response.status}`);
    return value;
  }
  async function refresh() {
    const data = await api<State>("state");
    if (mounted.current) setState(data);
  }
  async function connect() {
    setError("");
    try {
      await refresh();
      const evidence = await api<{ rows: Row[] }>("evidence");
      setRecorded(evidence.rows);
    } catch (e) {
      setError(String(e));
      setState(null);
    }
  }
  useEffect(() => {
    mounted.current = true;
    void connect();
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (tab !== "live") return;
    const id = setInterval(() => {
      void refresh().catch(() => {});
    }, 1200);
    return () => clearInterval(id);
  }, [tab, token]);
  async function action(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    setReceipt("");
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function replay() {
    setRows([]);
    await action(async () => {
      const data = await api<{ rows: Row[]; scope: string }>("replay", {
        trace,
        count: 4096,
      });
      setRows(data.rows);
      setScope(data.scope);
    });
  }
  async function query() {
    setResult(null);
    await action(async () => {
      setResult(await api<Get>(`get?key=${encodeURIComponent(key)}`));
      await refresh();
    });
  }
  async function mutate() {
    setResult(null);
    await action(async () => {
      const data = await api<{ version: number; invalidated: boolean }>(
        "mutate",
        { key, invalidate: announce },
      );
      setReceipt(
        `Origin ${key} is now v${data.version}. ${data.invalidated ? "Invalidation delivered." : "Invalidation deliberately withheld — stale data may be served."}`,
      );
      await refresh();
    });
  }
  async function fault() {
    await action(async () => {
      const next = !state?.origin_failing;
      await api("fault", { fail: next });
      setReceipt(
        next
          ? "Origin fault is active. Fresh cache entries remain usable; expired misses return an error."
          : "Origin recovered. Retry an expired or missing key.",
      );
      await refresh();
    });
  }
  const shown =
    tab === "evidence" ? recorded.filter((r) => r.trace === trace) : rows;
  const adaptive = shown.find((r) => r.mode === "adaptive");
  const baseline = shown.find((r) => r.mode === "plain-lru");
  return (
    <main>
      <header>
        <a href="/" className="logo" aria-label="Tidal home">
          <span className="logo-icon">↝</span> tidal
        </a>
        <div className="header-note">
          AN EXPERIMENT IN
          <br />
          TRAFFIC-LED CACHING
        </div>
        <div className="connection">
          <i className={state ? "on" : ""} />
          {state ? "LOCAL LAB CONNECTED" : "LAB DISCONNECTED"}
          <small>PUBLIC & SYNTHETIC DATA</small>
        </div>
      </header>
      <section className="hero">
        <div className="hero-type">
          <div className="eyebrow">CACHE LESS BLINDLY.</div>
          <h1>
            Let traffic
            <br />
            <span>choose.</span>
            <span className="h1-arrow">↳</span>
          </h1>
          <p>
            A cache that watches the pattern,
            <br />
            tests the alternatives, and changes its mind.
          </p>
        </div>
        <div className="ribbon-art" aria-hidden="true">
          <div className="art-caption">
            ONE STREAM.
            <br />
            THREE POSSIBLE FUTURES.
          </div>
          <div className="ribbon r1" />
          <div className="ribbon r2" />
          <div className="ribbon r3" />
          <div className="ribbon r4" />
          <div className="art-bottom">
            LRU / LFU / TINYLFU<span>→</span>
          </div>
        </div>
      </section>
      <nav aria-label="Lab views">
        {[
          ["replay", "01", "Replay lab"],
          ["live", "02", "Live cache"],
          ["evidence", "03", "Recorded runs"],
        ].map(([value, n, label]) => (
          <button
            key={value}
            aria-current={tab === value ? "page" : undefined}
            onClick={() => {
              setTab(value);
              setError("");
            }}
          >
            <span>{n}</span>
            {label}
            <b>↗</b>
          </button>
        ))}
      </nav>
      {error && (
        <div className="error" role="alert">
          <b>REQUEST REJECTED</b>
          {error}
        </div>
      )}
      {tab !== "live" ? (
        <>
          <section className="lab-heading">
            <div>
              <div className="eyebrow">
                {tab === "evidence"
                  ? "SAVED MEASUREMENTS / NOT LIVE TRAFFIC"
                  : "FIVE POLICIES. THE SAME TRAFFIC."}
              </div>
              <h2>
                {tab === "evidence"
                  ? "Keep the receipts."
                  : "Change the current."}
              </h2>
              <p>
                {tab === "evidence"
                  ? "Complete recorded campaign, including every policy that loses."
                  : "Replay a deterministic slice. Compare hits, origin work, and stale serves."}
              </p>
            </div>
            <div className="run-controls">
              <label htmlFor="trace">Traffic pattern</label>
              <select
                id="trace"
                value={trace}
                disabled={busy}
                onChange={(e) => {
                  setTrace(e.target.value);
                  setRows([]);
                }}
              >
                <option value="scan">Hot keys + one-hit scans</option>
                <option value="hotset">Stable hot set</option>
                <option value="shift">Shifting hot set</option>
                <option value="web12">Public Web12 key order</option>
              </select>
              {tab === "replay" && (
                <button
                  className="solid"
                  disabled={busy || !state}
                  onClick={() => void replay()}
                >
                  {busy ? "REPLAYING…" : "Run 4,096 requests"}
                  <span>↗</span>
                </button>
              )}
            </div>
          </section>
          <div className="scope">
            <span>
              {tab === "evidence" ? "RECORDED REPLAY" : "DETERMINISTIC REPLAY"}
            </span>
            <p>
              {tab === "evidence"
                ? "Full Web12 key sequence; synthetic clock, versions, TTL and invalidations. Equal object capacities, not equal total memory."
                : scope ||
                  "Synthetic clock / modeled origin / equal object capacities. Not live HTTP throughput."}
            </p>
          </div>
          {shown.length ? (
            <>
              <section className="score-strip">
                <div>
                  <span>ADAPTIVE HIT RATE</span>
                  <strong>{adaptive ? pct(adaptive.hit_rate) : "—"}</strong>
                  <small>
                    {adaptive ? fmt(adaptive.requests) : "—"} requests / this
                    trace
                  </small>
                </div>
                <div>
                  <span>VS PLAIN LRU</span>
                  <strong>
                    {adaptive && baseline
                      ? `${((adaptive.hit_rate - baseline.hit_rate) * 100).toFixed(1)} pp`
                      : "—"}
                  </strong>
                  <small>Positive helps. Negative stays visible.</small>
                </div>
                <div>
                  <span>ORIGIN WORK / REQUEST</span>
                  <strong>
                    {adaptive ? adaptive.origin_per_request.toFixed(3) : "—"}
                  </strong>
                  <small>Includes early refreshes, not just misses.</small>
                </div>
                <div>
                  <span>STALE SERVES</span>
                  <strong>{adaptive ? pct(adaptive.stale_rate) : "—"}</strong>
                  <small>Observed against synthetic version truth.</small>
                </div>
              </section>
              <section className="flow">
                <div className="section-line">
                  <h3>Hit rate, as the stream moves.</h3>
                  <span>Each column is a completed request window.</span>
                </div>
                <div
                  className="band-chart"
                  role="img"
                  aria-label="Hit rate by policy over time"
                >
                  {modes.map((mode, index) => {
                    const row = shown.find((r) => r.mode === mode);
                    return (
                      <div className={`policy-band color-${index}`} key={mode}>
                        <div className="band-name">
                          <b>{names[mode]}</b>
                          <span>{row ? pct(row.hit_rate) : "—"}</span>
                        </div>
                        <svg viewBox="0 0 900 80" preserveAspectRatio="none">
                          <path
                            d={
                              row
                                ? `M0,80 ${row.samples.map((s, i) => `L${(i / Math.max(1, row.samples.length - 1)) * 900},${80 - s.hits * 74}`).join(" ")} L900,80Z`
                                : ""
                            }
                          />
                          {[0, 1, 2, 3, 4, 5, 6, 7, 8].map((i) => (
                            <line
                              key={i}
                              x1={i * 112.5}
                              x2={i * 112.5}
                              y1="0"
                              y2="80"
                            />
                          ))}
                        </svg>
                      </div>
                    );
                  })}
                </div>
                <div className="chart-foot">
                  <span>0 REQUESTS</span>
                  <span>{shown[0]?.requests.toLocaleString()} REQUESTS →</span>
                </div>
              </section>
              <div className="results-table">
                <table>
                  <thead>
                    <tr>
                      <th>Policy</th>
                      <th>Hit rate</th>
                      <th>Origin calls</th>
                      <th>Early refreshes</th>
                      <th>Stale serves</th>
                      <th>Switches</th>
                    </tr>
                  </thead>
                  <tbody>
                    {shown.map((r) => (
                      <tr
                        className={r.mode === "adaptive" ? "active-row" : ""}
                        key={r.mode}
                      >
                        <th>{names[r.mode]}</th>
                        <td>{pct(r.hit_rate)}</td>
                        <td>{fmt(r.origin_calls)}</td>
                        <td>{fmt(r.refreshes)}</td>
                        <td>
                          {fmt(r.stale_serves)}{" "}
                          <small>({pct(r.stale_rate)})</small>
                        </td>
                        <td>{r.switches}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <section className="selection">
                <h3>The decisions behind the curve.</h3>
                <p>
                  Shadow caches never fetch. A challenger must win consecutive
                  windows by a margin before the active cache changes its
                  eviction rule.
                </p>
                <div className="switch-tape">
                  {adaptive?.windows.map((w) => (
                    <div className={w.switched ? "changed" : ""} key={w.at}>
                      <span>{fmt(w.at)}</span>
                      <b>{w.selected}</b>
                      <small>{w.switched ? "SWITCHED ↗" : "HELD →"}</small>
                    </div>
                  ))}
                </div>
              </section>
            </>
          ) : (
            <div className="empty">
              <span>↝</span>
              <h3>
                {tab === "evidence"
                  ? "No recorded measurements found."
                  : "A pattern is waiting to happen."}
              </h3>
              <p>
                {tab === "evidence"
                  ? "Run make bench to generate the public + synthetic campaign."
                  : "Choose a traffic pattern, then run a replay. These numbers will come from that run."}
              </p>
            </div>
          )}
        </>
      ) : (
        <>
          <section className="lab-heading">
            <div>
              <div className="eyebrow">REAL HTTP / LOCAL SYNTHETIC ORIGIN</div>
              <h2>Put it under pressure.</h2>
              <p>
                Query a key. Change its source. Break the origin. Watch what the
                cache actually serves.
              </p>
            </div>
            <div className="live-policy">
              <span>ACTIVE EVICTION RULE</span>
              <strong>{state?.cache.selected || "—"}</strong>
              <small>
                {state?.cache.switches || 0} measured switches since start
              </small>
            </div>
          </section>
          <section className="live-grid">
            <div className="query-panel">
              <div className="section-line">
                <h3>Send a request</h3>
                <span>GET /api/get</span>
              </div>
              <label htmlFor="key">Synthetic key</label>
              <div className="key-row">
                <input
                  id="key"
                  value={key}
                  disabled={busy}
                  onChange={(e) => {
                    setKey(e.target.value);
                    setResult(null);
                  }}
                />
                <button
                  className="solid"
                  disabled={busy || !state}
                  onClick={() => void query()}
                >
                  Fetch key ↗
                </button>
              </div>
              <p className="hint">
                k or p followed by up to five digits. No real records.
              </p>
              {result ? (
                <div
                  className={`query-receipt ${result.stale_observed ? "stale" : ""}`}
                  aria-live="polite"
                >
                  <div>
                    <b>{result.result.hit ? "CACHE HIT" : "ORIGIN FETCH"}</b>
                    <span>{result.elapsed_ms.toFixed(3)} ms</span>
                  </div>
                  <strong>
                    {result.result.value.key}{" "}
                    <small>v{result.result.value.version}</small>
                  </strong>
                  <p>{result.result.value.body}</p>
                  <span>
                    {result.stale_observed
                      ? "STALE OBSERVED — SOURCE VERSION IS NEWER"
                      : "MATCHES SYNTHETIC SOURCE VERSION"}
                  </span>
                </div>
              ) : (
                <div className="query-empty">The next response goes here.</div>
              )}
              <label className="check">
                <input
                  type="checkbox"
                  checked={announce}
                  disabled={busy}
                  onChange={(e) => setAnnounce(e.target.checked)}
                />{" "}
                Deliver invalidation after source change
              </label>
              <button
                className="line-button"
                disabled={busy || !state}
                onClick={() => void mutate()}
              >
                Change origin version →
              </button>
              <p className="hint">
                Withhold invalidation to expose the freshness limit. This is a
                deliberate negative control.
              </p>
            </div>
            <div className="traffic-panel">
              <div className="section-line">
                <h3>Drive the stream</h3>
                <span>REAL ORIGIN CALLS</span>
              </div>
              <label htmlFor="live-trace">Traffic pattern</label>
              <select
                id="live-trace"
                value={trace === "web12" ? "scan" : trace}
                disabled={busy}
                onChange={(e) => setTrace(e.target.value)}
              >
                <option value="scan">Hot keys + one-hit scans</option>
                <option value="hotset">Stable hot set</option>
                <option value="shift">Shifting hot set</option>
              </select>
              <button
                className="solid"
                disabled={busy || !state}
                onClick={() =>
                  void action(async () => {
                    await api("drive", {
                      trace: trace === "web12" ? "scan" : trace,
                      count: 1024,
                    });
                    await refresh();
                    setReceipt(
                      "1,024 real cache requests completed against the local HTTP origin.",
                    );
                  })
                }
              >
                {busy ? "DRIVING…" : "Send 1,024 requests ↗"}
              </button>
              <div className="live-counters">
                <div>
                  <span>REQUESTS</span>
                  <b>{fmt(state?.cache.requests || 0)}</b>
                </div>
                <div>
                  <span>CACHE HITS</span>
                  <b>{fmt(state?.cache.hits || 0)}</b>
                </div>
                <div>
                  <span>ORIGIN ATTEMPTS</span>
                  <b>{fmt(state?.cache.origin_calls || 0)}</b>
                </div>
                <div>
                  <span>EARLY REFRESHES</span>
                  <b>{fmt(state?.cache.refreshes || 0)}</b>
                </div>
              </div>
              <button
                className={`fault ${state?.origin_failing ? "fault-active" : ""}`}
                disabled={busy || !state}
                onClick={() => void fault()}
              >
                {state?.origin_failing
                  ? "● ORIGIN DOWN / Recover it"
                  : "↯ Inject an origin outage"}
              </button>
              <p className="hint">
                Fresh entries may still hit. Expired or missing entries must
                fail; no stale-on-error fallback.
              </p>
            </div>
          </section>
          {receipt && (
            <div className="receipt" role="status">
              {receipt}
            </div>
          )}
          <section className="shadow-section">
            <div className="section-line">
              <h3>Three shadows. No extra origin calls.</h3>
              <span>ADVISORY / FIXED TTL MODEL</span>
            </div>
            {state?.cache.windows.length ? (
              <>
                <div className="shadow-rates">
                  {["lru", "lfu", "tinylfu"].map((p) => (
                    <div key={p}>
                      <span>{p}</span>
                      <strong>
                        {pct(
                          state.cache.windows[state.cache.windows.length - 1]
                            .rates[p],
                        )}
                      </strong>
                      <small>last completed window</small>
                    </div>
                  ))}
                </div>
                <div className="switch-tape">
                  {state.cache.windows.map((w) => (
                    <div key={w.at} className={w.switched ? "changed" : ""}>
                      <span>{w.at}</span>
                      <b>{w.selected}</b>
                      <small>{w.switched ? "SWITCHED ↗" : "HELD →"}</small>
                    </div>
                  ))}
                </div>
              </>
            ) : (
              <p>
                Send traffic to complete a scoring window. No hypothetical hit
                rate is shown before measurement.
              </p>
            )}
          </section>
        </>
      )}
      <section className="fine-print">
        <b>THE SMALL PRINT IS PART OF THE PRODUCT.</b>
        <p>
          Adaptive does not mean always better. Shadow models advise eviction;
          they cannot predict future traffic or know about unannounced writes.
          TinyLFU here means sketch-based admission over LRU victims, not
          W-TinyLFU. Object capacity excludes policy metadata. Replay is not a
          service-throughput benchmark.
        </p>
        <details>
          <summary>Connection settings</summary>
          <label htmlFor="token">Bearer token</label>
          <div className="key-row">
            <input
              id="token"
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <button className="line-button" onClick={() => void connect()}>
              Reconnect
            </button>
          </div>
          <p className="hint">
            Default local-demo is loopback-only. The token is held in memory.
          </p>
        </details>
      </section>
      <footer>
        <span className="footer-logo">tidal ↝</span>
        <p>
          OWN THE POLICY.
          <br />
          ACCOUNT FOR THE COST.
        </p>
        <a
          href="https://github.com/rjrddy/tidal"
          target="_blank"
          rel="noreferrer"
        >
          Source + raw measurements ↗
        </a>
      </footer>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
