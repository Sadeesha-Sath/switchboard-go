import './style.css';
import {
  ApiError,
  fetchMetrics,
  fetchUsage,
  fetchWorkspaceUsage,
  reloadConfig,
  resetAllKeys,
  resetKey,
  validateKeys,
} from './api';
import type { AggregatedUsageResponse, MetricsSnapshot, WorkspaceUsageSnapshot } from './types';
import { renderPool } from './components/PoolUsage';
import { renderKeys } from './components/KeysTable';
import { renderMetrics } from './components/MetricsPanel';
import { renderServerInfo } from './components/SummaryCards';
import { renderWorkspaceUsage } from './components/WorkspaceUsage';
import { esc, fmtCountdown } from './utils';

declare global {
  interface Window {
    __SWB_CONFIG__?: { apiKey?: string };
  }
}

const embeddedKey: string = window.__SWB_CONFIG__?.apiKey ?? '';

interface Settings {
  baseUrl: string;
  apiKey: string;
  intervalMs: number;
}

const LS_BASE = 'sb_base_url';
const LS_KEY = 'sb_proxy_key';
const LS_INTERVAL = 'sb_interval_ms';

function loadSettings(): Settings {
  const raw = localStorage.getItem(LS_INTERVAL);
  const intervalMs = raw === null ? 30000 : Number(raw);
  return {
    baseUrl: localStorage.getItem(LS_BASE) ?? '',
    apiKey: localStorage.getItem(LS_KEY) ?? '',
    intervalMs: Number.isFinite(intervalMs) && intervalMs >= 0 ? intervalMs : 30000,
  };
}

let settings = loadSettings();
let pollTimer: number | undefined;
let isPolling = false;
let lastUsage: AggregatedUsageResponse | null = null;
let lastSnap: MetricsSnapshot | null = null;
let lastWorkspace: WorkspaceUsageSnapshot | null = null;

let selectedWorkspaceID: string | null = null;
let selectedWindowKey: 'rolling' | 'weekly' | 'monthly' = 'monthly';
let showAllModels = false;
let activeKeyMenuIndex: number | null = null;

const $ = <T extends HTMLElement>(sel: string): T => {
  const el = document.querySelector<T>(sel);
  if (!el) {
    throw new Error(`missing element ${sel}`);
  }
  return el;
};

function banner(msg: string | null): void {
  const el = $('#error-banner');
  if (msg) {
    el.textContent = msg;
    el.hidden = false;
  } else {
    el.hidden = true;
  }
}

function toast(msg: string): void {
  const el = $('#toast');
  el.textContent = msg;
  el.classList.add('show');
  window.setTimeout(() => el.classList.remove('show'), 3000);
}

function updateAutoControls(): void {
  const isAuto = settings.intervalMs > 0;
  const labelAuto = $('#label-auto-on');
  const labelHold = $('#label-auto-off');
  const radioAuto = $<HTMLInputElement>('#radio-auto-on');
  const radioHold = $<HTMLInputElement>('#radio-auto-off');
  const textAuto = $('#auto-label-text');

  if (isAuto) {
    labelAuto.classList.add('active');
    labelHold.classList.remove('active');
    radioAuto.checked = true;
    radioHold.checked = false;
    const sec = Math.round(settings.intervalMs / 1000);
    textAuto.textContent = `Auto ${sec}s`;
  } else {
    labelAuto.classList.remove('active');
    labelHold.classList.add('active');
    radioAuto.checked = false;
    radioHold.checked = true;
  }
}

function startPolling(): void {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer);
    pollTimer = undefined;
  }
  if (settings.intervalMs > 0 && settings.apiKey) {
    pollTimer = window.setInterval(() => void poll(false), settings.intervalMs);
  }
}

async function poll(forceRefresh: boolean, retried = false): Promise<void> {
  if (isPolling) return;
  if (!settings.apiKey) {
    banner('Set your proxy API key in Settings to load usage data.');
    return;
  }

  isPolling = true;
  const refreshBtn = $('#refresh-btn');
  refreshBtn.classList.add('loading');

  try {
    const [usage, snap, ws] = await Promise.all([
      fetchUsage(settings.baseUrl, settings.apiKey, forceRefresh),
      fetchMetrics(settings.baseUrl),
      fetchWorkspaceUsage(settings.baseUrl, settings.apiKey),
    ]);
    lastUsage = usage;
    lastSnap = snap;
    lastWorkspace = ws;
    banner(null);

    // Update updated time label
    const now = new Date();
    const timeStr = now.toISOString().slice(11, 19) + 'Z';
    $('#updated-label').textContent = `updated ${timeStr}`;

    render();
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      const usedOverride = settings.apiKey !== '' && settings.apiKey !== embeddedKey;
      if (usedOverride && embeddedKey && !retried) {
        localStorage.removeItem(LS_KEY);
        settings.apiKey = embeddedKey;
        $<HTMLInputElement>('#proxy-key').value = embeddedKey;
        isPolling = false;
        refreshBtn.classList.remove('loading');
        return poll(forceRefresh, true);
      }
      banner('Invalid proxy API key — update it in Settings.');
      openSettingsDialog();
    } else if (err instanceof ApiError) {
      banner(`Proxy request failed: ${err.message}`);
    } else {
      banner(`Cannot reach proxy at ${settings.baseUrl || window.location.origin}: ${String(err)}`);
    }
  } finally {
    isPolling = false;
    refreshBtn.classList.remove('loading');
  }
}

function render(): void {
  if (!lastUsage) {
    return;
  }

  // Header Subtitle Info
  $('#server-info').innerHTML = renderServerInfo(lastUsage, settings.baseUrl);

  // Pool Usage Section (3 columns + bottom summary line)
  $('#pool-section').innerHTML = renderPool(lastUsage, lastSnap);

  // Workspace Section
  const wsSection = $('#workspace-section');
  if (lastWorkspace && lastWorkspace.enabled && (lastWorkspace.workspaces.length > 0 || !!lastWorkspace.error)) {
    wsSection.hidden = false;
    wsSection.innerHTML = renderWorkspaceUsage(
      lastWorkspace,
      selectedWorkspaceID,
      selectedWindowKey,
      showAllModels,
    );
  } else {
    wsSection.hidden = true;
    wsSection.innerHTML = '';
  }

  // Keys Section
  $('#keys-section').innerHTML = renderKeys(lastUsage, activeKeyMenuIndex);

  // Traffic / Metrics Section
  if (lastSnap) {
    $('#traffic-section').innerHTML = renderMetrics(lastSnap, lastUsage);
  }

  // Footer Section
  const aliases = Object.entries(lastSnap?.model_aliases ?? {});
  const aliasList = aliases
    .map(([from, to]) => `${esc(from)} → ${esc(to)}`)
    .join(' · ');
  const aliasSummary = aliases.length > 0 ? `${aliases.length} aliases · ${aliasList}` : '';
  const metricsHref = `${settings.baseUrl || ''}/metrics`;
  const snapTime = lastSnap?.generated_at ? lastSnap.generated_at.slice(11, 19) + 'Z' : '—';

  $('#footer-section').innerHTML = `
    <div class="num" style="display:flex;justify-content:space-between;gap:var(--space-6);font-size:11.5px;color:var(--color-neutral-600);flex-wrap:wrap;line-height:1.9">
      <span>${aliasSummary}</span>
      <span>snapshot ${esc(snapTime)} · <a href="#" id="open-raw-json-link">raw JSON</a> · <a href="${esc(metricsHref)}" target="_blank" rel="noopener">/metrics</a></span>
    </div>
  `;
}

function tickCountdowns(): void {
  const now = Date.now();
  for (const el of document.querySelectorAll<HTMLElement>('[data-countdown]')) {
    el.textContent = fmtCountdown(el.dataset.countdown, now);
  }
  for (const el of document.querySelectorAll<HTMLElement>('[data-retry]')) {
    const n = Number(el.dataset.retry ?? 0);
    if (Number.isFinite(n) && n > 0) {
      el.dataset.retry = String(n - 1);
      el.textContent = `${n - 1}s`;
    }
  }
}

function openSettingsDialog(): void {
  const dlg = $<HTMLDialogElement>('#settings-dialog');
  $<HTMLInputElement>('#base-url').value = settings.baseUrl;
  $<HTMLInputElement>('#proxy-key').value = settings.apiKey;
  $<HTMLSelectElement>('#poll-interval').value = String(settings.intervalMs);
  if (typeof dlg.showModal === 'function') {
    dlg.showModal();
  } else {
    dlg.setAttribute('open', '');
  }
}

function closeSettingsDialog(): void {
  const dlg = $<HTMLDialogElement>('#settings-dialog');
  if (typeof dlg.close === 'function') {
    dlg.close();
  } else {
    dlg.removeAttribute('open');
  }
}

function openRawJsonDialog(): void {
  const dlg = $<HTMLDialogElement>('#raw-json-dialog');
  const pre = $('#raw-json-content');
  pre.textContent = lastSnap ? JSON.stringify(lastSnap, null, 2) : 'No snapshot data available.';
  if (typeof dlg.showModal === 'function') {
    dlg.showModal();
  } else {
    dlg.setAttribute('open', '');
  }
}

function closeRawJsonDialog(): void {
  const dlg = $<HTMLDialogElement>('#raw-json-dialog');
  if (typeof dlg.close === 'function') {
    dlg.close();
  } else {
    dlg.removeAttribute('open');
  }
}

async function handleAction(action: string | undefined, indexStr: string | undefined): Promise<void> {
  if (!action) return;
  // Close any open menus
  $('#dropdown-menu').hidden = true;
  activeKeyMenuIndex = null;

  try {
    switch (action) {
      case 'reset-key': {
        const index = Number(indexStr);
        if (!Number.isInteger(index)) return;
        await resetKey(settings.baseUrl, settings.apiKey, index);
        toast(`Key ${index} reset`);
        break;
      }
      case 'reset-all': {
        if (!window.confirm('Reset all keys and quota limits?')) {
          return;
        }
        await resetAllKeys(settings.baseUrl, settings.apiKey);
        toast('All keys reset');
        break;
      }
      case 'reload': {
        if (!window.confirm('Reload configuration from disk?')) {
          return;
        }
        await reloadConfig(settings.baseUrl, settings.apiKey);
        toast('Configuration reloaded');
        break;
      }
      case 'validate': {
        const res = await validateKeys(settings.baseUrl, settings.apiKey);
        const bad = res.results.filter((r) => r.state === 'exhausted').length;
        toast(`Validated ${res.results.length} keys (${bad} exhausted)`);
        break;
      }
      case 'settings': {
        openSettingsDialog();
        return;
      }
      default:
        return;
    }
    await poll(false);
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      banner('Invalid proxy API key — update it in Settings.');
    } else {
      toast(`Action failed: ${String(err)}`);
    }
  }
}

function bindEvents(): void {
  // Topbar Auto / Hold radio controls
  $('#label-auto-on').addEventListener('click', (e) => {
    e.preventDefault();
    if (settings.intervalMs <= 0) {
      settings.intervalMs = 30000;
      localStorage.setItem(LS_INTERVAL, String(settings.intervalMs));
    }
    updateAutoControls();
    startPolling();
    void poll(false);
  });

  $('#label-auto-off').addEventListener('click', (e) => {
    e.preventDefault();
    settings.intervalMs = 0;
    localStorage.setItem(LS_INTERVAL, '0');
    updateAutoControls();
    startPolling();
    toast('Auto-refresh paused (Hold)');
  });

  // Topbar Refresh now button
  $('#refresh-btn').addEventListener('click', () => {
    void poll(true);
  });

  // Topbar Actions Dropdown Menu button
  $('#menu-btn').addEventListener('click', (e) => {
    e.stopPropagation();
    const menu = $('#dropdown-menu');
    menu.hidden = !menu.hidden;
  });

  // Settings form submission
  $('#settings-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const newBase = $<HTMLInputElement>('#base-url').value;
    const newKey = $<HTMLInputElement>('#proxy-key').value;
    const newInterval = Number($<HTMLSelectElement>('#poll-interval').value);

    settings = {
      baseUrl: newBase,
      apiKey: newKey,
      intervalMs: Number.isFinite(newInterval) && newInterval >= 0 ? newInterval : 30000,
    };

    localStorage.setItem(LS_BASE, settings.baseUrl);
    localStorage.setItem(LS_KEY, settings.apiKey);
    localStorage.setItem(LS_INTERVAL, String(settings.intervalMs));

    closeSettingsDialog();
    updateAutoControls();
    startPolling();
    toast('Settings saved');
    void poll(false);
  });

  $('#clear-key').addEventListener('click', () => {
    localStorage.removeItem(LS_KEY);
    settings.apiKey = embeddedKey;
    $<HTMLInputElement>('#proxy-key').value = embeddedKey;
    toast('Proxy key override cleared');
  });

  $('#settings-close-btn').addEventListener('click', closeSettingsDialog);
  $('#settings-backdrop').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) {
      closeSettingsDialog();
    }
  });

  // Raw JSON modal handlers
  $('#raw-json-close-btn').addEventListener('click', closeRawJsonDialog);
  $('#raw-json-done-btn').addEventListener('click', closeRawJsonDialog);
  $('#raw-json-backdrop').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) {
      closeRawJsonDialog();
    }
  });

  $('#raw-json-copy-btn').addEventListener('click', async () => {
    const text = $('#raw-json-content').textContent ?? '';
    try {
      await navigator.clipboard.writeText(text);
      toast('Copied JSON to clipboard');
    } catch {
      toast('Failed to copy JSON');
    }
  });

  // Global document click listener for delegation and menu dismissals
  document.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;

    // Raw JSON open link
    if (target.closest('#open-raw-json-link')) {
      e.preventDefault();
      openRawJsonDialog();
      return;
    }

    // Menu item action trigger
    const actionEl = target.closest<HTMLElement>('[data-action]');
    if (actionEl) {
      const action = actionEl.dataset.action;
      const idx = actionEl.dataset.index;
      void handleAction(action, idx);
      return;
    }

    // Key actions menu trigger
    const keyMenuBtn = target.closest<HTMLElement>('[data-key-menu]');
    if (keyMenuBtn) {
      e.stopPropagation();
      const idx = Number(keyMenuBtn.dataset.keyMenu);
      activeKeyMenuIndex = activeKeyMenuIndex === idx ? null : idx;
      if (lastUsage) {
        $('#keys-section').innerHTML = renderKeys(lastUsage, activeKeyMenuIndex);
      }
      return;
    }

    // Workspace tab selector
    const wsTab = target.closest<HTMLElement>('[data-ws-id]');
    if (wsTab) {
      const id = wsTab.dataset.wsId ?? null;
      if (id && id !== selectedWorkspaceID) {
        selectedWorkspaceID = id;
        if (lastWorkspace) {
          $('#workspace-section').innerHTML = renderWorkspaceUsage(
            lastWorkspace,
            selectedWorkspaceID,
            selectedWindowKey,
            showAllModels,
          );
        }
      }
      return;
    }

    // Window tab selector (rolling / weekly / monthly)
    const winTab = target.closest<HTMLElement>('[data-win-key]');
    if (winTab) {
      const key = winTab.dataset.winKey as 'rolling' | 'weekly' | 'monthly';
      if (key && key !== selectedWindowKey) {
        selectedWindowKey = key;
        if (lastWorkspace) {
          $('#workspace-section').innerHTML = renderWorkspaceUsage(
            lastWorkspace,
            selectedWorkspaceID,
            selectedWindowKey,
            showAllModels,
          );
        }
      }
      return;
    }

    // Models expansion toggle
    const toggleModelsBtn = target.closest<HTMLElement>('#toggle-models-btn');
    if (toggleModelsBtn) {
      showAllModels = !showAllModels;
      if (lastWorkspace) {
        $('#workspace-section').innerHTML = renderWorkspaceUsage(
          lastWorkspace,
          selectedWorkspaceID,
          selectedWindowKey,
          showAllModels,
        );
      }
      return;
    }

    // Close topbar menu if clicked outside
    const menu = $('#dropdown-menu');
    if (!menu.hidden && !target.closest('#menu-btn') && !target.closest('#dropdown-menu')) {
      menu.hidden = true;
    }

    // Close key menu if clicked outside
    if (activeKeyMenuIndex !== null && !target.closest('.menu-container')) {
      activeKeyMenuIndex = null;
      if (lastUsage) {
        $('#keys-section').innerHTML = renderKeys(lastUsage, null);
      }
    }
  });

  // Tab visibility changes
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      if (pollTimer !== undefined) {
        window.clearInterval(pollTimer);
        pollTimer = undefined;
      }
    } else {
      startPolling();
      void poll(false);
    }
  });
}

function init(): void {
  updateAutoControls();
  bindEvents();
  window.setInterval(tickCountdowns, 1000);

  if (!settings.apiKey && embeddedKey) {
    settings.apiKey = embeddedKey;
  }

  if (settings.apiKey) {
    void poll(false);
  } else {
    openSettingsDialog();
    banner('Set your proxy API key in Settings to load usage data.');
  }

  startPolling();
}

init();
