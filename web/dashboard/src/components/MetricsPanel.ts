import type { AggregatedUsageResponse, MetricsSnapshot } from '../types';
import { esc, fmtAvgMs } from '../utils';

function hintFor(usage: AggregatedUsageResponse | null, keyIndex: number): string {
  const k = usage?.keys.find((x) => x.index === keyIndex);
  return k?.key_hint ?? `key #${keyIndex}`;
}

export function renderMetrics(snap: MetricsSnapshot, usage: AggregatedUsageResponse | null): string {
  // Group HTTP requests by `${method} ${endpoint}`
  const endpoints = new Map<
    string,
    {
      method: string;
      endpoint: string;
      totalCount: number;
      errors: Array<{ status: number; count: number }>;
    }
  >();

  for (const h of snap.http_requests) {
    const key = `${h.method} ${h.endpoint}`;
    let item = endpoints.get(key);
    if (!item) {
      item = {
        method: h.method,
        endpoint: h.endpoint,
        totalCount: 0,
        errors: [],
      };
      endpoints.set(key, item);
    }
    item.totalCount += h.count;
    if (h.status !== 200) {
      item.errors.push({ status: h.status, count: h.count });
    }
  }

  // Duration lookup map
  const durationMap = new Map<string, { sum: number; count: number }>();
  for (const d of snap.http_durations) {
    durationMap.set(`${d.method} ${d.endpoint}`, {
      sum: d.duration_seconds_sum,
      count: d.duration_seconds_count,
    });
  }

  const sortedEndpoints = Array.from(endpoints.values()).sort(
    (a, b) => b.totalCount - a.totalCount,
  );

  const trafficRows: string[] = [];

  for (const ep of sortedEndpoints) {
    const dur = durationMap.get(`${ep.method} ${ep.endpoint}`);
    const avgMs = dur && dur.count > 0 ? ` · ${fmtAvgMs(dur.sum, dur.count)}` : '';
    trafficRows.push(`
      <div class="traffic-row">
        <span>${esc(ep.method)} ${esc(ep.endpoint)}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${ep.totalCount}${avgMs}</span>
      </div>
    `);

    for (const err of ep.errors) {
      trafficRows.push(`
        <div class="traffic-row">
          <span style="color:var(--color-accent-300);padding-left:12px">└ ${err.status}</span>
          <span style="color:var(--color-neutral-300)">${err.count}</span>
        </div>
      `);
    }
  }

  // Upstream traffic per key
  const upstreamSorted = [...snap.upstream_requests].sort(
    (a, b) => a.key_index - b.key_index || b.count - a.count,
  );

  for (const u of upstreamSorted) {
    const hint = hintFor(usage, u.key_index);
    const avgMs = u.duration_seconds_count > 0 ? ` · ${fmtAvgMs(u.duration_seconds_sum, u.duration_seconds_count)}` : '';
    const statusNote = u.status !== 200 ? ` (${u.status})` : '';

    trafficRows.push(`
      <div class="traffic-row">
        <span>upstream · ${esc(hint)} pri ${u.priority}${statusNote}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${u.count}${avgMs}</span>
      </div>
    `);
  }

  const content =
    trafficRows.length > 0
      ? trafficRows.join('')
      : `<div class="text-muted">No requests recorded yet.</div>`;

  return `
    <div>
      <div class="kicker" style="margin-bottom:var(--space-3)">Traffic · API routes</div>
      <div class="num traffic-lines">
        ${content}
      </div>
    </div>
  `;
}
