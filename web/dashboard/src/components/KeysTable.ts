import type { AggregatedUsageResponse, PerKeyUsage } from '../types';
import { esc, fmtLocalTime, fmtPct, fmtTimeAgo, usageClass } from '../utils';

function stateChip(k: PerKeyUsage): string {
  const cls = k.state === 'available' ? 'ok' : k.state === 'exhausted' ? 'bad' : 'unknown';
  return `<span class="chip ${cls}">${esc(k.state)}</span>`;
}

function winCell(win: PerKeyUsage['rolling'], threshold: number): string {
  const cls = usageClass(win.percent, threshold);
  const reset = win.resetsAt
    ? `<span class="muted small" data-countdown="${esc(win.resetsAt)}">${esc(fmtLocalTime(win.resetsAt))}</span>`
    : '';
  return `
    <td class="win-cell">
      <div class="bar tiny"><div class="bar-fill ${cls}" style="width:${Math.min(100, Math.max(0, win.percent))}%"></div></div>
      <span class="win-pct">${fmtPct(win.percent)}</span>
      ${reset}
    </td>
  `;
}

function retryCell(k: PerKeyUsage): string {
  if (k.retry_after_seconds !== undefined) {
    return `<td><span class="chip bad" data-retry="${k.retry_after_seconds}">${k.retry_after_seconds}s</span></td>`;
  }
  return `<td><span class="muted">—</span></td>`;
}

export function renderKeys(usage: AggregatedUsageResponse): string {
  const threshold = usage.summary.proactive_threshold_percent || 95;
  const rows = usage.keys
    .map(
      (k) => `
      <tr class="${k.current ? 'current-row' : ''}" title="${esc(k.error ?? '')}">
        <td>${k.index}${k.current ? ' <span class="star" title="current key">★</span>' : ''}</td>
        <td class="mono">${esc(k.key_hint ?? '…')}</td>
        <td>${k.priority} / ${k.weight}</td>
        <td>${stateChip(k)}</td>
        <td>${k.eligible ? '<span class="ok-text">✔</span>' : '<span class="bad-text">✘</span>'}</td>
        ${winCell(k.rolling, threshold)}
        ${winCell(k.weekly, threshold)}
        ${winCell(k.monthly, threshold)}
        ${retryCell(k)}
        <td class="muted small" title="${esc(k.last_checked_at ?? '')}">${esc(fmtTimeAgo(k.last_checked_at))}</td>
        <td><button class="btn tiny-btn" data-action="reset-key" data-index="${k.index}">Reset</button></td>
      </tr>
    `,
    )
    .join('');

  return `
    <div class="section-head">
      <h2>Keys</h2>
      <div class="actions">
        <button class="btn" data-action="validate">Validate keys</button>
        <button class="btn" data-action="reset-all">Reset all</button>
        <button class="btn" data-action="reload">Reload config</button>
      </div>
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>#</th><th>Key</th><th>Pri/W</th><th>State</th><th>Elig</th>
            <th>Rolling</th><th>Weekly</th><th>Monthly</th><th>Retry</th><th>Checked</th><th></th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
  `;
}
