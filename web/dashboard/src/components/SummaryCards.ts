import type { AggregatedUsageResponse } from '../types';
import { esc, fmtCountdown, fmtPct } from '../utils';

export function renderSummary(usage: AggregatedUsageResponse): string {
  const s = usage.summary;
  const nextReset = usage.rolling.resetsAt;
  return `
    <div class="card">
      <div class="card-value">${s.total_keys}</div>
      <div class="card-label">Keys</div>
      <div class="card-sub"><span class="chip ok">${s.available_keys} avail</span> <span class="chip ${s.exhausted_keys > 0 ? 'bad' : 'muted-chip'}">${s.exhausted_keys} exhausted</span></div>
    </div>
    <div class="card">
      <div class="card-value">${s.active_sessions}</div>
      <div class="card-label">Active sessions</div>
    </div>
    <div class="card">
      <div class="card-value small-value">${esc(s.routing_strategy)}</div>
      <div class="card-label">Routing strategy</div>
    </div>
    <div class="card">
      <div class="card-value">${fmtPct(s.proactive_threshold_percent)}</div>
      <div class="card-label">Proactive threshold</div>
    </div>
    <div class="card">
      <div class="card-value small-value" data-countdown="${esc(nextReset ?? '')}">${esc(fmtCountdown(nextReset))}</div>
      <div class="card-label">Next rolling reset</div>
    </div>
  `;
}
