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
import { renderSummary } from './components/SummaryCards';
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
  const intervalMs = raw === null ? 10000 : Number(raw);
  return {
    baseUrl: localStorage.getItem(LS_BASE) ?? '',
    apiKey: localStorage.getItem(LS_KEY) ?? '',
    intervalMs: Number.isFinite(intervalMs) && intervalMs >= 0 ? intervalMs : 10000,
  };
}

let settings = loadSettings();
let pollTimer: number | undefined;
let lastUsage: AggregatedUsageResponse | null = null;
let lastSnap: MetricsSnapshot | null = null;
let lastWorkspace: WorkspaceUsageSnapshot | null = null;
let selectedWorkspaceID: string | null = null;

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
  let el = document.querySelector<HTMLDivElement>('#toast');
  if (!el) {
    el = document.createElement('div');
    el.id = 'toast';
    document.body.appendChild(el);
  }
  el.textContent = msg;
  el.classList.add('show');
  window.setTimeout(() => el.classList.remove('show'), 3000);
}

function applyHeader(): void {
  const label = $('#base-url-label');
  label.textContent = settings.baseUrl || window.location.origin;
  $<HTMLSelectElement>('#interval').value = String(settings.intervalMs);
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
  if (!settings.apiKey) {
    banner('Set your proxy API key in Settings to load usage data.');
    return;
  }
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
    render();
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      const usedOverride =
        settings.apiKey !== '' && settings.apiKey !== embeddedKey;
      if (usedOverride && embeddedKey && !retried) {
        localStorage.removeItem(LS_KEY);
        settings.apiKey = embeddedKey;
        $<HTMLInputElement>('#proxy-key').value = embeddedKey;
        return poll(forceRefresh, true);
      }
      banner('Invalid proxy API key — update it in Settings.');
      $('#settings').hidden = false;
    } else if (err instanceof ApiError) {
      banner(`Proxy request failed: ${err.message}`);
    } else {
      banner(`Cannot reach proxy at ${settings.baseUrl || window.location.origin}: ${String(err)}`);
    }
  }
}

function render(): void {
  if (!lastUsage || !lastSnap) {
    return;
  }
  $('#summary').innerHTML = renderSummary(lastUsage);
  $('#pool').innerHTML = renderPool(lastUsage);
  $('#keys').innerHTML = renderKeys(lastUsage);
  $('#metrics').innerHTML = renderMetrics(lastSnap, lastUsage);

  const wsSection = $('#workspace-usage');
  if (lastWorkspace && lastWorkspace.enabled && (lastWorkspace.workspaces.length > 0 || !!lastWorkspace.error)) {
    wsSection.hidden = false;
    wsSection.innerHTML = renderWorkspaceUsage(lastWorkspace, selectedWorkspaceID);
  } else {
    wsSection.hidden = true;
    wsSection.innerHTML = '';
  }

  const aliases = Object.entries(lastSnap.model_aliases ?? {});
  const chips = aliases
    .map(([from, to]) => `<span class="alias-chip mono">${esc(from)} → ${esc(to)}</span>`)
    .join('');
  const metricsHref = `${settings.baseUrl || ''}/metrics`;
  $('#footer').innerHTML = `
    ${chips ? `<div class="aliases">${chips}</div>` : ''}
    <div class="muted small">Snapshot ${esc(lastSnap.generated_at)} · raw Prometheus: <a href="${esc(metricsHref)}">/metrics</a></div>
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

function bindEvents(): void {
  $('#settings-toggle').addEventListener('click', () => {
    const el = $('#settings');
    el.hidden = !el.hidden;
  });

  $('#settings-form').addEventListener('submit', (e) => {
    e.preventDefault();
    settings = {
      baseUrl: $<HTMLInputElement>('#base-url').value,
      apiKey: $<HTMLInputElement>('#proxy-key').value,
      intervalMs: settings.intervalMs,
    };
    localStorage.setItem(LS_BASE, settings.baseUrl);
    localStorage.setItem(LS_KEY, settings.apiKey);
    $('#settings').hidden = true;
    applyHeader();
    startPolling();
    void poll(false);
  });

  $('#clear-key').addEventListener('click', () => {
    localStorage.removeItem(LS_KEY);
    settings.apiKey = embeddedKey;
    $<HTMLInputElement>('#proxy-key').value = embeddedKey;
    banner('Proxy key override cleared.');
  });

  $('#interval').addEventListener('change', () => {
    settings.intervalMs = Number($<HTMLSelectElement>('#interval').value);
    localStorage.setItem(LS_INTERVAL, String(settings.intervalMs));
    startPolling();
  });

  $('#refresh').addEventListener('click', () => void poll(true));

  document.addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLElement>('[data-action]');
    if (!btn) {
      return;
    }
    void handleAction(btn.dataset.action, btn.dataset.index);
  });

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

  $('#workspace-usage').addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLElement>('[data-ws-tab]');
    if (!btn) {
      return;
    }
    selectedWorkspaceID = btn.dataset.wsTab ?? null;
    if (lastWorkspace) {
      $('#workspace-usage').innerHTML = renderWorkspaceUsage(lastWorkspace, selectedWorkspaceID);
    }
  });
}

async function handleAction(action: string | undefined, indexStr: string | undefined): Promise<void> {
  if (!action) {
    return;
  }
  try {
    switch (action) {
      case 'reset-key': {
        const index = Number(indexStr);
        if (!Number.isInteger(index)) {
          return;
        }
        await resetKey(settings.baseUrl, settings.apiKey, index);
        toast(`Key ${index} reset`);
        break;
      }
      case 'reset-all': {
        if (!window.confirm('Mark all keys as available?')) {
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

function init(): void {
  applyHeader();
  $<HTMLInputElement>('#base-url').value = settings.baseUrl;
  $<HTMLInputElement>('#proxy-key').value = settings.apiKey;
  bindEvents();
  window.setInterval(tickCountdowns, 1000);
  if (!settings.apiKey && embeddedKey) {
    settings.apiKey = embeddedKey;
  }
  if (settings.apiKey) {
    void poll(false);
  } else {
    $('#settings').hidden = false;
    banner('Set your proxy API key in Settings to load usage data.');
  }
  startPolling();
}

init();
