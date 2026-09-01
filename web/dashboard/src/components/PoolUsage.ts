import type { AggregatedUsageResponse, SummaryWindow, UsageWindow } from '../types';
import { esc, fmtCountdown, fmtPct, usageClass } from '../utils';

function poolBar(name: string, win: UsageWindow, sum: SummaryWindow | undefined, earliestReset?: string): string {
  const cls = usageClass(win.percent, 95);
  const minMax = sum ? `${fmtPct(sum.min_percent)} – ${fmtPct(sum.max_percent)}` : '—';
  const remaining = sum ? fmtPct(sum.total_remaining_percent) : '—';
  const reset =
    earliestReset ?? win.resetsAt
      ? `<span class="muted" data-countdown="${esc(earliestReset ?? win.resetsAt ?? '')}">${esc(fmtCountdown(earliestReset ?? win.resetsAt))}</span>`
      : '<span class="muted">—</span>';
  return `
    <div class="pool-row">
      <div class="pool-head">
        <span class="pool-name">${esc(name)}</span>
        <span class="pool-avg">${fmtPct(win.percent)} avg</span>
        ${reset}
      </div>
      <div class="bar"><div class="bar-fill ${cls}" style="width:${Math.min(100, Math.max(0, win.percent))}%"></div></div>
      <div class="pool-sub muted small">min–max ${esc(minMax)} · remaining ${esc(remaining)}</div>
    </div>
  `;
}

export function renderPool(usage: AggregatedUsageResponse): string {
  const p = usage.summary.pool_usage;
  return `
    <h2>Pool usage</h2>
    ${poolBar('Rolling', usage.rolling, p?.rolling, p?.rolling?.earliest_reset_at)}
    ${poolBar('Weekly', usage.weekly, p?.weekly)}
    ${poolBar('Monthly', usage.monthly, p?.monthly)}
  `;
}
