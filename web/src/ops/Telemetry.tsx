import { useEffect, useState, useCallback } from "react";
import { Link } from "react-router";
import { api, type TelemetryResponse, type TelemetryWindow } from "../api/client";
import "../styles/telemetry.css";

// Telemetry is the Phase J UI — aggregated view of the async
// mcp_tool_calls pipeline. Self-contained page (not a Reader Surface)
// because ops data has a different shape and audience than content.
//
// Reads are polling + manual refresh rather than streaming because a
// missed tool call in the UI is a non-issue; data lives in the DB and
// the next refresh catches up.

const WINDOWS: { value: TelemetryWindow; label: string }[] = [
  { value: "1h", label: "1h" },
  { value: "6h", label: "6h" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
];

export function Telemetry() {
  const [data, setData] = useState<TelemetryResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [window, setWindow] = useState<TelemetryWindow>("24h");
  const [autoRefresh, setAutoRefresh] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const r = await api.telemetry({ window });
      setData(r);
    } catch (e) {
      setErr(String(e));
    } finally {
      setLoading(false);
    }
  }, [window]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!autoRefresh) return;
    const id = setInterval(load, 10_000);
    return () => clearInterval(id);
  }, [autoRefresh, load]);

  const totals = data?.totals;
  const errorRate = totals && totals.calls > 0 ? (totals.errors / totals.calls) : 0;

  return (
    <div className="ops">
      <header className="ops__bar">
        <Link to="/" className="ops__back">◀ Reader</Link>
        <h1 className="ops__title">MCP Telemetry</h1>
        <div className="ops__controls">
          <div className="ops__windows">
            {WINDOWS.map((w) => (
              <button
                key={w.value}
                type="button"
                className={`ops__win ${window === w.value ? "is-active" : ""}`}
                onClick={() => setWindow(w.value)}
              >
                {w.label}
              </button>
            ))}
          </div>
          <label className="ops__auto">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            <span>auto</span>
          </label>
          <button type="button" className="ops__refresh" onClick={load} disabled={loading}>
            {loading ? "…" : "refresh"}
          </button>
        </div>
      </header>

      {err && <div className="ops__error">{err}</div>}

      {totals && (
        <section className="ops__totals">
          <Metric label="calls" value={totals.calls.toLocaleString()} />
          <Metric label="errors" value={totals.errors.toLocaleString()} sub={`${(errorRate * 100).toFixed(1)}%`} emphasize={totals.errors > 0} />
          <Metric label="in tokens" value={totals.total_input_tokens.toLocaleString()} />
          <Metric label="out tokens" value={totals.total_output_tokens.toLocaleString()} />
          <Metric label="total tokens" value={(totals.total_input_tokens + totals.total_output_tokens).toLocaleString()} emphasize />
          <Metric label="agents" value={totals.unique_agents.toLocaleString()} />
        </section>
      )}

      {data && data.tools.length === 0 && !err && (
        <div className="ops__empty">
          No tool calls in the last {window}. Either no MCP sessions ran
          in this window or the telemetry pipeline isn't wired — check
          the server log for "telemetry flush failed".
        </div>
      )}

      {data && data.tools.length > 0 && (
        <section className="ops__tools">
          <h2>Per tool</h2>
          <table>
            <thead>
              <tr>
                <th>tool</th>
                <th className="num">calls</th>
                <th className="num">errs</th>
                <th className="num">avg ms</th>
                <th className="num">p95 ms</th>
                <th className="num">avg in tok</th>
                <th className="num">avg out tok</th>
                <th className="num">total tokens</th>
                <th>last</th>
              </tr>
            </thead>
            <tbody>
              {data.tools.map((t) => {
                const totalTok = t.total_input_tokens + t.total_output_tokens;
                return (
                  <tr key={t.tool_name}>
                    <td className="tool">{t.tool_name}</td>
                    <td className="num">{t.calls}</td>
                    <td className={`num ${t.errors > 0 ? "err" : ""}`}>{t.errors || "·"}</td>
                    <td className="num">{t.avg_duration_ms}</td>
                    <td className="num">{t.p95_duration_ms}</td>
                    <td className="num">{t.avg_input_tokens}</td>
                    <td className="num">{t.avg_output_tokens}</td>
                    <td className="num strong">{totalTok.toLocaleString()}</td>
                    <td className="ts">{formatRelative(t.last_call_at)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </section>
      )}

      {data && data.recent.length > 0 && (
        <section className="ops__recent">
          <h2>Recent calls ({data.recent.length})</h2>
          <table>
            <thead>
              <tr>
                <th>time</th>
                <th>tool</th>
                <th className="num">ms</th>
                <th className="num">in</th>
                <th className="num">out</th>
                <th>error</th>
                <th>agent</th>
              </tr>
            </thead>
            <tbody>
              {data.recent.map((c, i) => (
                <tr key={`${c.started_at}-${i}`} className={c.error_code ? "is-err" : ""}>
                  <td className="ts">{formatRelative(c.started_at)}</td>
                  <td className="tool">{c.tool_name}</td>
                  <td className="num">{c.duration_ms}</td>
                  <td className="num" title={`${c.input_bytes}B`}>{c.input_tokens_est}t</td>
                  <td className="num" title={`${c.output_bytes}B`}>{c.output_tokens_est}t</td>
                  <td className="err">{c.error_code || "·"}</td>
                  <td className="mono">{c.agent_id ? c.agent_id.slice(0, 10) : "·"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      <footer className="ops__foot">
        <span>Token counts are approximations (tiktoken cl100k_base). Actual Claude billing may differ ±20% on CJK content.</span>
      </footer>
    </div>
  );
}

function Metric({ label, value, sub, emphasize }: { label: string; value: string; sub?: string; emphasize?: boolean }) {
  return (
    <div className={`ops__metric ${emphasize ? "is-strong" : ""}`}>
      <div className="ops__metric-val">{value}</div>
      <div className="ops__metric-lab">{label}{sub && <span className="ops__metric-sub"> · {sub}</span>}</div>
    </div>
  );
}

function formatRelative(iso: string): string {
  const d = new Date(iso);
  const sec = Math.max(0, Math.floor((Date.now() - d.getTime()) / 1000));
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  const day = Math.floor(hr / 24);
  return `${day}d`;
}
