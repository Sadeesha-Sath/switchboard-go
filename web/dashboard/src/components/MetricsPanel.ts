import type { AggregatedUsageResponse, MetricsSnapshot } from '../types';
import { esc, fmtAvgMs } from '../utils';

function hintFor(usage: AggregatedUsageResponse | null, keyIndex: number): string {
  const k = usage?.keys.find((x) => x.index === keyIndex);
  return k?.key_hint ?? `key ${keyIndex}`;
}

export function renderMetrics(snap: MetricsSnapshot, usage: AggregatedUsageResponse | null): string {
  const http = [...snap.http_requests].sort((a, b) => b.count - a.count);
  const httpRows = http.length
    ? http
        .map(
          (h) => `
        <tr>
          <td class="mono">${esc(h.endpoint)}</td>
          <td>${esc(h.method)}</td>
          <td>${h.status === 200 ? '<span class="ok-text">200</span>' : h.status >= 500 ? '<span class="bad-text">' + h.status + '</span>' : h.status}</td>
          <td>${h.count}</td>
        </tr>`,
        )
        .join('')
    : '<tr><td colspan="4" class="muted">No requests recorded yet</td></tr>';

  const latRows = snap.http_durations.length
    ? snap.http_durations
        .map(
          (d) => `
        <tr>
          <td class="mono">${esc(d.endpoint)}</td>
          <td>${esc(d.method)}</td>
          <td>${esc(fmtAvgMs(d.duration_seconds_sum, d.duration_seconds_count))}</td>
          <td>${d.duration_seconds_count}</td>
        </tr>`,
        )
        .join('')
    : '<tr><td colspan="4" class="muted">No latency data yet</td></tr>';

  const upstream = [...snap.upstream_requests].sort((a, b) => a.key_index - b.key_index || b.count - a.count);
  const upRows = upstream.length
    ? upstream
        .map(
          (u) => `
        <tr>
          <td class="mono">${esc(hintFor(usage, u.key_index))}</td>
          <td>${u.priority}</td>
          <td>${u.status === 200 ? '<span class="ok-text">200</span>' : u.status === 429 ? '<span class="bad-text">429</span>' : u.status}</td>
          <td>${u.count}</td>
          <td>${esc(fmtAvgMs(u.duration_seconds_sum, u.duration_seconds_count))}</td>
        </tr>`,
        )
        .join('')
    : '<tr><td colspan="5" class="muted">No upstream traffic yet</td></tr>';

  const exRows = snap.key_exhaustions.length
    ? snap.key_exhaustions
        .map(
          (e) =>
            `<tr><td class="mono">${esc(hintFor(usage, e.key_index))}</td><td>${e.count}</td></tr>`,
        )
        .join('')
    : '<tr><td colspan="2" class="muted">None 🎉</td></tr>';

  const swRows = snap.key_switches.length
    ? snap.key_switches
        .map(
          (s) =>
            `<tr><td>${s.from_key} → ${s.to_key}</td><td>${esc(s.reason)}</td><td>${s.count}</td></tr>`,
        )
        .join('')
    : '<tr><td colspan="3" class="muted">None</td></tr>';

  return `
    <h2>Metrics</h2>
    <div class="metrics-grid">
      <div class="metric-block">
        <h3>Requests by endpoint</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Status</th><th>Count</th></tr></thead><tbody>${httpRows}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Latency (avg)</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Avg</th><th>Count</th></tr></thead><tbody>${latRows}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Upstream traffic per key</h3>
        <table><thead><tr><th>Key</th><th>Pri</th><th>Status</th><th>Count</th><th>Avg</th></tr></thead><tbody>${upRows}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Exhaustions</h3>
        <table><thead><tr><th>Key</th><th>429s</th></tr></thead><tbody>${exRows}</tbody></table>
        <h3>Key switches</h3>
        <table><thead><tr><th>Switch</th><th>Reason</th><th>Count</th></tr></thead><tbody>${swRows}</tbody></table>
      </div>
    </div>
    <details class="raw-metrics">
      <summary>Raw snapshot JSON</summary>
      <pre>${esc(JSON.stringify(snap, null, 2))}</pre>
    </details>
  `;
}
