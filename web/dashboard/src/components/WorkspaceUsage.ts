import type { WorkspaceStatus, WorkspaceUsageSnapshot, WorkspaceWindowSnapshot } from '../types';
import { esc } from '../utils';

export const WINDOW_LABELS: Record<string, string> = {
  rolling: '5-hour',
  weekly: 'Weekly',
  monthly: 'Monthly',
};

function fmtUSD(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(amount);
}

function renderWindow(win: WorkspaceWindowSnapshot, label: string): string {
  const pct = Math.min(100, Math.max(0, win.usage_percent));
  const resetsAt = new Date(Date.now() + win.reset_in_sec * 1000).toISOString();
  const rows = win.rows
    .map(
      (r) => `<tr>
        <td>${esc(r.name)}${r.estimated ? ' <span class="muted small">(est.)</span>' : ''}</td>
        <td class="mono">${fmtUSD(r.cost)}</td>
        <td class="mono">${fmtUSD(r.quota_cost)}</td>
        <td class="mono">${r.contribution_percent.toFixed(1)}%</td>
      </tr>`,
    )
    .join('');
  const table = rows
    ? `<table>
        <thead><tr><th>Model</th><th>Cost</th><th>Quota cost</th><th>Contribution</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>`
    : '<p class="muted small">No model usage recorded in this window.</p>';
  return `
    <div class="ws-window" data-component="ws-window">
      <div class="ws-window-head">
        <span>${esc(label)}</span>
        <span class="mono">${win.usage_percent.toFixed(1)}% · ${fmtUSD(win.usage_usd)} / ${fmtUSD(win.limit_usd)}</span>
      </div>
      <div class="progress" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100">
        <div class="progress-bar" style="width:${pct}%"></div>
      </div>
      <div class="muted small">Resets in <span data-countdown="${esc(resetsAt)}"></span></div>
      ${table}
    </div>`;
}

function renderWorkspace(ws: WorkspaceStatus): string {
  if (ws.error) {
    return `<p class="muted small">Workspace ${esc(ws.name)}: ${esc(ws.error)}</p>`;
  }
  return ['rolling', 'weekly', 'monthly']
    .map((k) => {
      const win = ws.windows[k];
      if (!win) {
        return '';
      }
      return renderWindow(win, WINDOW_LABELS[k] ?? k);
    })
    .join('');
}

export function renderWorkspaceUsage(snap: WorkspaceUsageSnapshot, selectedID: string | null): string {
  if (!snap.enabled) {
    return '';
  }
  const tabs = snap.workspaces
    .map(
      (ws) =>
        `<button class="ws-tab${ws.id === selectedID ? ' active' : ''}" data-ws-tab="${esc(ws.id)}">${esc(ws.name)}</button>`,
    )
    .join('');
  const selected = snap.workspaces.find((ws) => ws.id === selectedID) ?? snap.workspaces[0];
  const body = selected
    ? renderWorkspace(selected)
    : `<p class="muted small">No workspaces found for this account.</p>`;
  const err = snap.error ? `<p class="muted small">Last refresh error: ${esc(snap.error)}</p>` : '';
  return `
    <div class="ws-tabs">${tabs}</div>
    ${err}
    ${body}
    ${snap.updated_at ? `<div class="muted small">Updated ${esc(snap.updated_at)}</div>` : ''}`;
}
