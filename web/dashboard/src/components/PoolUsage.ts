import type { AggregatedUsageResponse, MetricsSnapshot, SummaryWindow, UsageWindow } from '../types';
import { esc, fmtCountdown, fmtPct, usageClass } from '../utils';

function renderPoolCard(
  title: string,
  win: UsageWindow,
  sum: SummaryWindow | undefined,
  earliestReset?: string,
  accentReset = false,
): string {
  const avg = sum ? sum.average_percent : win.percent;
  const pct = Math.min(100, Math.max(0, avg));
  const cls = usageClass(pct, 95);
  const minMax = sum ? `${fmtPct(sum.min_percent)} – ${fmtPct(sum.max_percent)}` : '—';
  const remaining = sum ? `${Math.round(sum.total_remaining_percent)}%` : '—';
  const resetIso = earliestReset ?? win.resetsAt;
  const resetText = resetIso ? fmtCountdown(resetIso) : '—';
  const resetClass = accentReset ? 'color: var(--color-accent-400);' : 'color: var(--color-neutral-400);';

  return `
    <div>
      <div style="display:flex;align-items:baseline;gap:var(--space-2)">
        <span class="num big">${fmtPct(avg)}</span>
        <span class="num nw" style="font-size:12px;color:var(--color-neutral-500)">avg used</span>
        <span class="kicker" style="margin-left:auto;font-size:10.5px;color:var(--color-neutral-500)">${esc(title)}</span>
      </div>
      <div class="bar-track" style="margin:var(--space-4) 0 var(--space-3)">
        <div class="bar-fill ${cls}" style="width:${Math.max(1, pct)}%"></div>
      </div>
      <div class="num nw" style="font-size:12px;color:var(--color-neutral-500);display:flex;justify-content:space-between;gap:var(--space-3);flex-wrap:wrap">
        <span>min–max ${esc(minMax)}</span>
        <span>remaining ${esc(remaining)}</span>
      </div>
      <div class="num" style="font-size:13px;${resetClass}margin-top:var(--space-3);white-space:nowrap">
        earliest reset ${resetIso ? `in <span data-countdown="${esc(resetIso)}">${esc(resetText)}</span>` : '—'}
      </div>
    </div>
  `;
}

export function renderPool(usage: AggregatedUsageResponse, snap: MetricsSnapshot | null): string {
  const s = usage.summary;
  const p = s.pool_usage;
  const totalKeys = s.total_keys;
  const availKeys = s.available_keys;
  const exhaustKeys = s.exhausted_keys;
  const activeSessions = s.active_sessions;

  const totalExhaustions = snap ? snap.key_exhaustions.reduce((acc, x) => acc + x.count, 0) : 0;
  const totalSwitches = snap ? snap.key_switches.reduce((acc, x) => acc + x.count, 0) : 0;

  return `
    <div class="pool-header">
      <span class="kicker">Pool usage · averaged across ${totalKeys} ${totalKeys === 1 ? 'key' : 'keys'}</span>
      <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);letter-spacing:.04em;white-space:nowrap">
        warn ≥ 70% · critical ≥ 95%
      </span>
    </div>

    <div class="pool">
      ${renderPoolCard('5-hour', usage.rolling, p?.rolling, p?.rolling?.earliest_reset_at ?? usage.rolling.resetsAt, true)}
      ${renderPoolCard('Weekly', usage.weekly, p?.weekly, p?.weekly?.earliest_reset_at ?? usage.weekly.resetsAt, false)}
      ${renderPoolCard('Monthly', usage.monthly, p?.monthly, p?.monthly?.earliest_reset_at ?? usage.monthly.resetsAt, false)}
    </div>

    <div class="summary-line num">
      <span><span style="color:var(--color-neutral-100)">${totalKeys}</span> ${totalKeys === 1 ? 'key' : 'keys'} · ${availKeys} available · ${exhaustKeys} exhausted</span>
      <span><span style="color:var(--color-neutral-100)">${activeSessions}</span> active sessions</span>
      <span><span style="color:var(--color-neutral-100)">${totalExhaustions}</span> exhaustions · ${totalSwitches} switches</span>
      <span class="summary-legend">
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-neutral-400)"></span>ok</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-500)"></span>warn</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-300)"></span>critical</span>
      </span>
    </div>
  `;
}
