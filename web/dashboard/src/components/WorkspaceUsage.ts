import type { WorkspaceUsageSnapshot, WorkspaceWindowSnapshot } from '../types';
import { esc, fmtDuration, fmtPct, fmtUSD, usageClass } from '../utils';

export const WINDOW_KEYS: Array<'rolling' | 'weekly' | 'monthly'> = ['rolling', 'weekly', 'monthly'];

export const WINDOW_LABELS: Record<string, string> = {
  rolling: '5-hour',
  weekly: 'Weekly',
  monthly: 'Monthly',
};

export function renderWorkspaceUsage(
  snap: WorkspaceUsageSnapshot,
  selectedID: string | null,
  selectedWin: 'rolling' | 'weekly' | 'monthly',
  showAll: boolean,
): string {
  if (!snap.enabled) {
    return '';
  }

  if (snap.error && snap.workspaces.length === 0) {
    return `
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <div class="banner">Workspace telemetry error: ${esc(snap.error)}</div>
      </div>
    `;
  }

  const workspaces = snap.workspaces;
  if (workspaces.length === 0) {
    return `
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <p class="text-muted" style="font-size:13px;">No workspace telemetry configured or recorded.</p>
      </div>
    `;
  }

  const currentWs = workspaces.find((ws) => ws.id === selectedID) ?? workspaces[0];
  const activeWsId = currentWs.id;

  // Render workspace selector tabs
  const wsTabs = workspaces
    .map((ws) => {
      const active = ws.id === activeWsId;
      return `
        <label class="segl num ${active ? 'active' : ''}" data-ws-id="${esc(ws.id)}">
          <input type="radio" name="ws-select" ${active ? 'checked' : ''} />
          ${esc(ws.name)}
        </label>
      `;
    })
    .join('');

  // Render window selector tabs
  const winTabs = WINDOW_KEYS.map((k) => {
    const active = k === selectedWin;
    return `
      <label class="segl num ${active ? 'active' : ''}" data-win-key="${k}">
        <input type="radio" name="win-select" ${active ? 'checked' : ''} />
        ${WINDOW_LABELS[k]}
      </label>
    `;
  }).join('');

  if (currentWs.error) {
    return `
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${wsTabs}</div>
        </div>
        <div class="banner">Workspace ${esc(currentWs.name)}: ${esc(currentWs.error)}</div>
      </div>
    `;
  }

  const win: WorkspaceWindowSnapshot | undefined = currentWs.windows[selectedWin];
  if (!win) {
    return `
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${wsTabs}</div>
          <div class="seg-group" style="margin-left:auto">${winTabs}</div>
        </div>
        <p class="text-muted" style="font-size:13px;">No data recorded for this window.</p>
      </div>
    `;
  }

  const usd = fmtUSD(win.usage_usd);
  const limit = fmtUSD(win.limit_usd);
  const pctUsed = win.usage_percent;
  const left = fmtUSD(Math.max(0, win.limit_usd - win.usage_usd));
  const barCls = usageClass(pctUsed, 95);
  const barW = Math.min(100, Math.max(0.8, pctUsed));
  const resetDuration = fmtDuration(win.reset_in_sec);
  const rows = win.rows || [];
  const countLabel = rows.length > 0 ? `${rows.length} models charged` : 'no model usage recorded';
  const maxContrib = rows.length ? Math.max(...rows.map((r) => r.contribution_percent)) || 1 : 1;

  const rest = rows.slice(8);
  const hasMore = rest.length > 0;
  const visibleRows = showAll ? rows : rows.slice(0, 8);

  const rowsMarkup = visibleRows
    .map((r) => {
      const mult = r.multiplier && r.multiplier !== 1 ? `${r.multiplier}×` : '';
      const est = r.estimated ? ' (est.)' : '';
      const barWidth = Math.max(1.5, (r.contribution_percent / maxContrib) * 100);
      const avail = fmtUSD(Math.max(0, win.limit_usd - r.quota_cost));

      return `
        <div class="mrow data-row">
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-neutral-200)">
            ${esc(r.name)}
            ${mult ? `<span class="num" style="font-size:11px;color:var(--color-accent-400);margin-left:4px;">${esc(mult)}</span>` : ''}
            ${est ? `<span class="num" style="font-size:11px;color:var(--color-neutral-600);margin-left:2px;">${esc(est)}</span>` : ''}
          </span>
          <span style="display:flex;align-items:center;gap:var(--space-2)">
            <span style="flex:1;height:3px;background:rgba(243,242,242,.14);display:block;overflow:hidden">
              <span style="display:block;height:3px;background:var(--color-accent-400);width:${barWidth}%"></span>
            </span>
            <span class="num" style="font-size:12.5px;color:var(--color-neutral-200);width:44px;text-align:right">
              ${fmtPct(r.contribution_percent)}
            </span>
          </span>
          <span class="num" style="text-align:right;color:var(--color-neutral-500)">${fmtUSD(r.cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-neutral-200)">${fmtUSD(r.quota_cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-accent-300)">${avail}</span>
        </div>
      `;
    })
    .join('');

  const restQuotaSum = rest.reduce((sum, r) => sum + r.quota_cost, 0);
  const moreBtnText = showAll
    ? 'Show top 8 only'
    : `Show ${rest.length} more · ${fmtUSD(restQuotaSum)} quota cost`;
  const chevronPath = showAll ? 'M18 15l-6-6-6 6' : 'M6 9l6 6 6-6';

  const footerText =
    rows.length === 0
      ? 'No model usage recorded in this window.'
      : `${showAll ? rows.length : Math.min(8, rows.length)} of ${rows.length} models shown.<br>Available = window limit − that model's quota cost.`;

  return `
    <div>
      <div class="ws-head-controls">
        <span class="kicker">Workspace</span>
        <div class="seg-group" id="ws-tabs-group">${wsTabs}</div>
        <div class="seg-group" id="win-tabs-group" style="margin-left:auto">${winTabs}</div>
      </div>

      <div class="ws-big-stats">
        <span class="num ws-big-amount">${usd}</span>
        <span class="num" style="font-size:13px;color:var(--color-neutral-500)">of ${limit} · ${fmtPct(pctUsed)} used</span>
        <span class="num" style="margin-left:auto;font-size:13px;color:var(--color-accent-300);white-space:nowrap">${left} available</span>
      </div>

      <div class="bar-track" style="margin:var(--space-3) 0 var(--space-3)">
        <div class="bar-fill ${barCls}" style="width:${barW}%"></div>
      </div>

      <div class="num" style="font-size:12px;color:var(--color-neutral-500)">
        Resets in ${resetDuration} · status ${esc(win.status || 'ok')} · ${countLabel}
      </div>

      <div class="mrow header-row kicker">
        <span>Model</span>
        <span>Contribution</span>
        <span style="text-align:right">Cost</span>
        <span style="text-align:right">Quota cost</span>
        <span style="text-align:right">Available</span>
      </div>

      <div id="ws-model-rows">${rowsMarkup}</div>

      ${
        hasMore
          ? `
        <button type="button" id="toggle-models-btn" class="num btn-outline-gold">
          <span>${esc(moreBtnText)}</span>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:block">
            <path d="${chevronPath}"></path>
          </svg>
        </button>
      `
          : ''
      }

      <div class="num" style="font-size:12px;color:var(--color-neutral-500);margin-top:var(--space-3);line-height:1.7">
        ${footerText}
      </div>
    </div>
  `;
}
