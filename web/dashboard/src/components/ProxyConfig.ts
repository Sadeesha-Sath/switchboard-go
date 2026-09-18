import type {
  ProxyConfigKeyPatch,
  ProxyConfigPatch,
  ProxyConfigResponse,
  ProxyConfigSettings,
} from '../types';
import { esc } from '../utils';

const DURATION_RE = /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/;

const DURATION_FIELDS: Array<{ name: keyof ProxyConfigSettings; id: string; label: string }> = [
  { name: 'session_ttl', id: 'cfg-session-ttl', label: 'Session TTL' },
  { name: 'balanced_idle_timeout', id: 'cfg-balanced-idle-timeout', label: 'Balanced idle timeout' },
  { name: 'retry_exhausted_after', id: 'cfg-retry-exhausted-after', label: 'Retry exhausted after' },
];

const STRATEGIES = ['session_sticky', 'balanced', 'round_robin', 'fill_first'];

function envBadge(locked: Set<string>, name: string): string {
  return locked.has(name)
    ? '<span class="env-badge" title="Overridden by an environment variable">env</span>'
    : '';
}

function disabled(locked: Set<string>, name: string): string {
  return locked.has(name) ? ' disabled' : '';
}

function keyRow(row: { key_hint?: string; id?: number; priority?: number; weight?: number } | null): string {
  const isNew = row === null;
  const id = isNew ? '' : String(row.id);
  const hint = isNew ? 'new key' : row.key_hint || '…';
  const secret = isNew
    ? '<input type="password" class="input" data-key-value placeholder="sk-…" autocomplete="new-password" />'
    : '<input type="password" class="input" data-key-rotate placeholder="rotate (optional)" autocomplete="new-password" />';
  return `
    <div class="cfg-key-row" data-key-row>
      <input type="hidden" data-key-id value="${esc(id)}" />
      <span class="num cfg-key-hint">${esc(hint)}</span>
      <label class="num cfg-inline">pri
        <input type="number" class="input" min="1" data-key-priority value="${row?.priority ?? 1}" />
      </label>
      <label class="num cfg-inline">w
        <input type="number" class="input" min="1" data-key-weight value="${row?.weight ?? 1}" />
      </label>
      ${secret}
      <button type="button" class="btn btn-secondary btn-xs" data-remove-key>Remove</button>
    </div>
  `;
}

function aliasRow(from: string, to: string, original: string): string {
  return `
    <div class="cfg-alias-row" data-alias-row data-alias-original="${esc(original)}">
      <input type="text" class="input" data-alias-from placeholder="alias" value="${esc(from)}" />
      <input type="text" class="input" data-alias-to placeholder="target model" value="${esc(to)}" />
      <button type="button" class="btn btn-secondary btn-xs" data-remove-alias>Remove</button>
    </div>
  `;
}

export function renderNewKeyRow(): string {
  return keyRow(null);
}

export function renderNewAliasRow(): string {
  return aliasRow('', '', '');
}

export function renderProxyConfigForm(cfg: ProxyConfigResponse): string {
  const locked = new Set(cfg.env_locked);
  const s = cfg.settings;
  const strategyOptions = STRATEGIES.map(
    (v) => `<option value="${v}"${s.routing_strategy === v ? ' selected' : ''}>${v}</option>`,
  ).join('');

  const durationFields = DURATION_FIELDS.map(
    ({ name, id, label }) => `
      <div class="field">
        <label for="${id}">${label}${envBadge(locked, name)}</label>
        <input type="text" id="${id}" class="input" value="${esc(String(s[name]))}"${disabled(locked, name)} />
      </div>`,
  ).join('');

  const keyRows = locked.has('keys')
    ? '<p class="text-muted cfg-note">Keys are managed by OPENCODE_GO_API_KEYS.</p>'
    : cfg.keys.map((k) => keyRow(k)).join('');

  const aliasRows = locked.has('model_aliases')
    ? ''
    : Object.entries(cfg.model_aliases)
        .map(([from, to]) => aliasRow(from, to, from))
        .join('');

  return `
    <div class="cfg-section">
      <div class="kicker">Routing</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-routing-strategy">Strategy${envBadge(locked, 'routing_strategy')}</label>
          <select id="cfg-routing-strategy" class="input"${disabled(locked, 'routing_strategy')}>${strategyOptions}</select>
        </div>
        <div class="field">
          <label for="cfg-proactive-threshold">Proactive threshold %${envBadge(locked, 'proactive_switch_threshold')}</label>
          <input type="number" id="cfg-proactive-threshold" class="input" min="0" max="100" step="0.1" value="${s.proactive_switch_threshold}"${disabled(locked, 'proactive_switch_threshold')} />
        </div>
        ${durationFields}
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Polling</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-usage-check-interval">Usage check interval${envBadge(locked, 'usage_check_interval')}</label>
          <input type="text" id="cfg-usage-check-interval" class="input" value="${esc(s.usage_check_interval)}"${disabled(locked, 'usage_check_interval')} />
        </div>
        <div class="field">
          <label for="cfg-disable-polling">Disable usage polling${envBadge(locked, 'disable_usage_polling')}</label>
          <input type="checkbox" id="cfg-disable-polling"${s.disable_usage_polling ? ' checked' : ''}${disabled(locked, 'disable_usage_polling')} />
        </div>
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Models</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-sanitize-role">Sanitize developer role${envBadge(locked, 'sanitize_developer_role')}</label>
          <input type="checkbox" id="cfg-sanitize-role"${s.sanitize_developer_role ? ' checked' : ''}${disabled(locked, 'sanitize_developer_role')} />
        </div>
      </div>
      <div class="kicker" style="margin-top:var(--space-4)">Model aliases${envBadge(locked, 'model_aliases')}</div>
      <div data-alias-list>${aliasRows}</div>
      <button type="button" class="btn btn-secondary" data-add-alias${locked.has('model_aliases') ? ' disabled' : ''}>Add alias</button>
    </div>
    <div class="cfg-section">
      <div class="kicker">Upstream keys${envBadge(locked, 'keys')}</div>
      <div data-key-list>${keyRows}</div>
      <button type="button" class="btn btn-secondary" data-add-key${locked.has('keys') ? ' disabled' : ''}>Add key</button>
      <p class="text-muted cfg-note">Existing key values never leave the server. Paste a new value to rotate a key.</p>
    </div>
  `;
}

function readInput(root: HTMLElement, selector: string): HTMLInputElement | null {
  return root.querySelector<HTMLInputElement>(selector);
}

export function collectProxyConfigPatch(
  baseline: ProxyConfigResponse,
  root: HTMLElement,
): { patch?: ProxyConfigPatch; error?: string; changed: boolean } {
  const locked = new Set(baseline.env_locked);
  const b = baseline.settings;
  const patch: ProxyConfigPatch = { if_revision: baseline.revision };
  const settings: Record<string, string | number | boolean | null> = {};

  const strategy = root.querySelector<HTMLSelectElement>('#cfg-routing-strategy');
  if (strategy && !locked.has('routing_strategy') && strategy.value !== b.routing_strategy) {
    settings.routing_strategy = strategy.value;
  }

  const threshold = readInput(root, '#cfg-proactive-threshold');
  if (threshold && !locked.has('proactive_switch_threshold')) {
    const value = Number(threshold.value);
    if (!Number.isFinite(value) || value < 0 || value > 100) {
      return { error: 'proactive_switch_threshold must be between 0 and 100', changed: false };
    }
    if (value !== b.proactive_switch_threshold) {
      settings.proactive_switch_threshold = value;
    }
  }

  for (const { name, id } of DURATION_FIELDS) {
    if (locked.has(name)) continue;
    const input = readInput(root, `#${id}`);
    if (!input) continue;
    const value = input.value.trim();
    if (!DURATION_RE.test(value)) {
      return { error: `${name} must be a duration like 30s, 5m, or 2h`, changed: false };
    }
    if (value !== b[name]) {
      settings[name] = value;
    }
  }

  const usageInterval = readInput(root, '#cfg-usage-check-interval');
  if (usageInterval && !locked.has('usage_check_interval')) {
    const value = usageInterval.value.trim();
    if (!DURATION_RE.test(value)) {
      return { error: 'usage_check_interval must be a duration like 30s, 5m, or 2h', changed: false };
    }
    if (value !== b.usage_check_interval) {
      settings.usage_check_interval = value;
    }
  }

  const disablePolling = readInput(root, '#cfg-disable-polling');
  if (disablePolling && !locked.has('disable_usage_polling') && disablePolling.checked !== b.disable_usage_polling) {
    settings.disable_usage_polling = disablePolling.checked;
  }

  const sanitize = readInput(root, '#cfg-sanitize-role');
  if (sanitize && !locked.has('sanitize_developer_role') && sanitize.checked !== b.sanitize_developer_role) {
    settings.sanitize_developer_role = sanitize.checked;
  }

  if (Object.keys(settings).length > 0) {
    patch.settings = settings;
  }

  if (!locked.has('model_aliases')) {
    const aliases: Record<string, string | null> = {};
    const seen = new Set<string>();
    for (const row of root.querySelectorAll<HTMLElement>('[data-alias-row]')) {
      const from = row.querySelector<HTMLInputElement>('[data-alias-from]')?.value.trim() ?? '';
      const to = row.querySelector<HTMLInputElement>('[data-alias-to]')?.value.trim() ?? '';
      const original = row.dataset.aliasOriginal ?? '';
      if (!from) {
        if (original) aliases[original] = null;
        continue;
      }
      if (seen.has(from)) {
        return { error: `duplicate alias ${from}`, changed: false };
      }
      seen.add(from);
      if (original && original !== from) {
        aliases[original] = null;
      }
      if (!to) {
        if (baseline.model_aliases[from] !== undefined) {
          aliases[from] = null;
        }
        continue;
      }
      if (baseline.model_aliases[from] !== to) {
        aliases[from] = to;
      }
    }
    if (Object.keys(aliases).length > 0) {
      patch.model_aliases = aliases;
    }
  }

  if (!locked.has('keys')) {
    const keys: ProxyConfigKeyPatch[] = [];
    for (const row of root.querySelectorAll<HTMLElement>('[data-key-row]')) {
      const idRaw = row.querySelector<HTMLInputElement>('[data-key-id]')?.value ?? '';
      const priority = Number(row.querySelector<HTMLInputElement>('[data-key-priority]')?.value);
      const weight = Number(row.querySelector<HTMLInputElement>('[data-key-weight]')?.value);
      if (!Number.isInteger(priority) || priority < 1) {
        return { error: 'key priority must be >= 1', changed: false };
      }
      if (!Number.isInteger(weight) || weight < 1) {
        return { error: 'key weight must be >= 1', changed: false };
      }
      if (idRaw === '') {
        const value = row.querySelector<HTMLInputElement>('[data-key-value]')?.value.trim() ?? '';
        if (!value) {
          return { error: 'new keys need a value', changed: false };
        }
        keys.push({ key: value, priority, weight });
      } else {
        const entry: ProxyConfigKeyPatch = { id: Number(idRaw), priority, weight };
        const rotate = row.querySelector<HTMLInputElement>('[data-key-rotate]')?.value.trim();
        if (rotate) {
          entry.key = rotate;
        }
        keys.push(entry);
      }
    }
    if (keys.length === 0) {
      return { error: 'at least one upstream key is required', changed: false };
    }
    patch.keys = keys;
  }

  const changed =
    patch.settings !== undefined || patch.model_aliases !== undefined || patch.keys !== undefined;
  if (!changed) {
    return { changed: false };
  }
  return { patch, changed: true };
}
