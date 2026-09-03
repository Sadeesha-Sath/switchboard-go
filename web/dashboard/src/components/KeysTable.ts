import type { AggregatedUsageResponse, PerKeyUsage, UsageWindow } from '../types';
import { esc, fmtCountdown, fmtPct, fmtTimeAgo, usageClass } from '../utils';

function renderMiniBar(label: string, win: UsageWindow, threshold: number): string {
  const pct = Math.min(100, Math.max(0, win.percent));
  const cls = usageClass(pct, threshold);
  const resetIso = win.resetsAt;
  const resetText = resetIso ? fmtCountdown(resetIso) : '—';

  return `
    <div style="flex:1">
      <div class="num klab">
        <span>${esc(label)}</span>
        <span style="color:var(--color-neutral-300)">${fmtPct(pct)}</span>
      </div>
      <div style="height:3px;background:rgba(243,242,242,.14);margin-top:7px;border-radius:1px;overflow:hidden">
        <div class="bar-fill ${cls}" style="width:${Math.max(1, pct)}%;height:3px"></div>
      </div>
      <div class="num" style="font-size:10.5px;color:var(--color-neutral-600);margin-top:6px">
        ${resetIso ? `<span data-countdown="${esc(resetIso)}">${esc(resetText)}</span>` : '—'}
      </div>
    </div>
  `;
}

function renderKeyCard(k: PerKeyUsage, threshold: number, activeMenuIndex: number | null): string {
  const isCurrent = k.current;
  const tagClass =
    k.state === 'available'
      ? 'tag-available'
      : k.state === 'exhausted'
        ? 'tag-exhausted'
        : 'tag-unknown';

  const retryText =
    k.retry_after_seconds !== undefined
      ? `<span style="color:var(--status-bad)" data-retry="${k.retry_after_seconds}">${k.retry_after_seconds}s</span>`
      : '—';

  const eligibleIcon = k.eligible
    ? `<span style="color:var(--color-accent-400);display:flex">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M20 6 9 17l-5-5"></path>
         </svg>
       </span>`
    : `<span style="color:var(--status-bad);display:flex;font-size:12px;line-height:1">✕</span>`;

  const starIcon = isCurrent
    ? `<span style="color:var(--color-accent-400);display:flex" title="current key">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"></path>
         </svg>
       </span>`
    : '';

  const isMenuOpen = activeMenuIndex === k.index;

  return `
    <div class="key-card ${isCurrent ? 'current' : ''}" title="${esc(k.error ?? '')}">
      <div class="key-card-header">
        <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);width:12px">${k.index}</span>
        ${starIcon}
        <span class="num key-hint">${esc(k.key_hint ?? '…')}</span>
        <span class="num tag ${tagClass}">${esc(k.state)}</span>
        <div class="menu-container" style="margin-left:auto;">
          <button type="button" class="iconbtn sm" data-key-menu="${k.index}" aria-label="Key ${k.index} actions" title="Key ${k.index} actions">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="display:block">
              <circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
              <circle cx="19" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
              <circle cx="5" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
            </svg>
          </button>
          <div class="dropdown-menu" id="key-menu-${k.index}" ${isMenuOpen ? '' : 'hidden'}>
            <div class="menuitem accent-item" data-action="reset-key" data-index="${k.index}">Reset key quota</div>
          </div>
        </div>
      </div>

      <div class="num key-meta-line">
        <span>pri ${k.priority} / w ${k.weight}</span>
        <span style="display:inline-flex;align-items:center;gap:5px">eligible ${eligibleIcon}</span>
        <span>retry ${retryText}</span>
        <span title="${esc(k.last_checked_at ?? '')}">checked ${esc(fmtTimeAgo(k.last_checked_at))}</span>
      </div>

      <div class="krow">
        ${renderMiniBar('5-HOUR', k.rolling, threshold)}
        ${renderMiniBar('WEEKLY', k.weekly, threshold)}
        ${renderMiniBar('MONTHLY', k.monthly, threshold)}
      </div>
    </div>
  `;
}

export function renderKeys(usage: AggregatedUsageResponse, activeKeyMenu: number | null = null): string {
  const threshold = usage.summary.proactive_threshold_percent || 95;
  const cards = usage.keys.map((k) => renderKeyCard(k, threshold, activeKeyMenu)).join('');

  return `
    <div>
      <div class="kicker" style="margin-bottom:var(--space-4)">Keys</div>
      ${cards || '<p class="text-muted" style="font-size:13px;">No keys configured.</p>'}
    </div>
  `;
}
