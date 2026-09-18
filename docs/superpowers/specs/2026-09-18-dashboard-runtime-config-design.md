# Dashboard runtime configuration

Date: 2026-09-18
Status: approved for planning

## Problem

Changing routing strategy, thresholds, aliases, or the upstream key list means
editing `config.yaml` and sending SIGHUP. The dashboard can reload config from
disk, but it cannot change values. We want to edit the core proxy settings in
the dashboard and have them apply immediately and survive a restart.

## Goals

- Edit core proxy settings and the upstream key list from the dashboard.
- Apply changes to the running process without a restart.
- Persist changes to the config file that the process already uses.
- Never send upstream key values back to the browser.
- Keep env overrides authoritative.

## Non-goals

- Editing `listen_addr`, `proxy_api_key`, `upstream.base_url`,
  `max_request_body_bytes`, alerts, SMTP, or workspace usage settings. Those
  stay file/env only.
- Showing full upstream key values.
- Config history, versioning, per-user roles, or audit logging beyond one log
  line per apply.
- Live push to the browser. The dashboard keeps polling.

## Editable fields

| JSON name | YAML path | Env override | Default |
| --- | --- | --- | --- |
| `routing_strategy` | `upstream.routing_strategy` | `ROUTING_STRATEGY` | `session_sticky` |
| `session_ttl` | `upstream.session_ttl` | `SESSION_TTL` | `2h` |
| `balanced_idle_timeout` | `upstream.balanced_idle_timeout` | `BALANCED_IDLE_TIMEOUT` | `1h` |
| `usage_check_interval` | `upstream.usage_check_interval` | `USAGE_CHECK_INTERVAL` | `30s` |
| `disable_usage_polling` | `upstream.disable_usage_polling` | `DISABLE_USAGE_POLLING` | `false` |
| `proactive_switch_threshold` | `upstream.proactive_switch_threshold` | `PROACTIVE_SWITCH_THRESHOLD` | `95.0` |
| `retry_exhausted_after` | `upstream.retry_exhausted_after` | `RETRY_EXHAUSTED_AFTER` | `5m` |
| `sanitize_developer_role` | `transformations.sanitize_developer_role` | `SANITIZE_DEVELOPER_ROLE` | `true` |
| `model_aliases` | `models.aliases` | `MODEL_ALIASES` | `{}` |
| `keys` | `upstream.api_keys` | `OPENCODE_GO_API_KEYS` | must be non-empty |

A field is env-locked when its env var is set to a non-empty value. Duration
fields are Go duration strings, matching the YAML file.

## Admin API

Both endpoints sit under the existing `/admin/` auth check. The request must
carry the proxy API key as `Authorization: Bearer`.

### GET /admin/config

```json
{
  "config_source": "/Users/x/.config/switchboard-go/config.yaml",
  "editable": true,
  "revision": "1f0a…",
  "env_locked": ["routing_strategy"],
  "settings": {
    "routing_strategy": "session_sticky",
    "session_ttl": "2h",
    "balanced_idle_timeout": "1h",
    "usage_check_interval": "30s",
    "disable_usage_polling": false,
    "proactive_switch_threshold": 95,
    "retry_exhausted_after": "5m",
    "sanitize_developer_role": true
  },
  "model_aliases": { "gpt-4o": "glm-5.1" },
  "keys": [
    { "id": 0, "key_hint": "sk-abcd…1234", "priority": 1, "weight": 3 }
  ]
}
```

- `config_source` is the path PATCH will write, or `"none"` when no path can be
  resolved.
- `editable` is false when no writable config path can be resolved. Permission or disk failures surface on PATCH as 500. Settings and masked keys are still returned.
- `env_locked` lists JSON field names from the table above, plus `model_aliases`
  and `keys` when their env vars are set. Values in the response are always the
  effective values after env overrides.
- `revision` is the hex SHA-256 of one canonical JSON document holding
  `settings`, `model_aliases` with sorted keys, and `keys` in list order where
  each entry is `{"key_sha256":…,"priority":…,"weight":…}`. The plaintext key
  never enters the hash input, so rotating a key with the same hint still
  changes the revision.
- `keys[].id` is the current list index. It is only stable between a GET and the
  next PATCH.

### PATCH /admin/config

Request bodies use JSON Merge Patch semantics for `settings` and
`model_aliases`, plus a list convention for `keys`.

```json
{
  "if_revision": "1f0a…",
  "settings": {
    "routing_strategy": "balanced",
    "session_ttl": null
  },
  "model_aliases": {
    "gpt-4o": null,
    "claude-sonnet": "glm-5.1"
  },
  "keys": [
    { "id": 0, "priority": 2 },
    { "id": 1, "key": "sk-rotated-value" },
    { "key": "sk-new-key", "priority": 1, "weight": 2 }
  ]
}
```

Rules:

- Every top-level field is optional. Omitted fields stay unchanged.
- `settings`: merge. A `null` value resets that field to its built-in default.
  Unknown setting names are errors.
- `model_aliases`: merge. A `null` value deletes the alias.
- `keys`, when present, is the complete desired list.
  - Entry with `id` edits that key. Omitted `priority` or `weight` keep their
    current value. A non-empty `key` rotates the secret. An empty `key` string
    is an error.
  - Entry with no `id` adds a key and must include a non-empty `key`. Omitted
    `priority` or `weight` default to 1.
  - Existing ids not present in the list are removed.
  - Duplicate ids, duplicate resulting key values, and an empty resulting list
    are errors.
  - Key strings are trimmed before storage.
- `if_revision` is optional. When present and stale, the server returns 409 and
  changes nothing. The dashboard always sends it.
- Unknown top-level fields are errors.
- Success returns `200` with the same body shape as GET.
- The body must be JSON. The dashboard sends `Content-Type: application/json`.

### Errors

| Status | Cause | Code |
| --- | --- | --- |
| 400 | malformed JSON, unknown field, invalid value, duplicate keys, validation failure | `invalid_config` |
| 409 | stale `if_revision` | `config_revision_conflict` |
| 409 | patch touches an env-locked field | `config_env_locked` |
| 412 | `editable` is false | `config_not_editable` |
| 500 | file write or apply failure | `config_apply_failed` |

Errors use the existing `writeAPIError` JSON shape. The message names the
offending field, for example `session_ttl must be a valid duration >= 0`.

## Persistence

Write path resolution, in order:

1. `cfg.ConfigSourcePath`, the file the process loaded at startup.
2. `SWITCHBOARD_GO_CONFIG`, if set. Startup already requires it to exist.
3. `~/.config/switchboard-go/config.yaml`, created on first write with `0700`
   for the directory and `0600` for the file.

If the home directory cannot be resolved and no file exists, `editable` is
false and PATCH returns 412.

Writing steps:

1. Parse the existing file into a `yaml.Node` tree. Comments, key order, and
   keys this feature does not manage are preserved.
2. Update only the paths in the editable-fields table. When a section or key
   does not exist, append it. Touched key entries are written in the structured
   form (`key`, `priority`, `weight`).
3. If the alias map becomes empty, remove `models.aliases`, and remove `models`
   if it becomes empty.
4. Write to a temp file in the same directory, `fsync`, then `rename` over the
   original. The temp file gets the existing file's mode, or `0600` for a new
   file.
5. Run `loadConfig()` again so the effective config matches what a restart
   would produce. Env precedence stays intact.

When the process started from env only, the created file contains just the
edited paths. Required values keep coming from env, and the file is partial by
design. The startup validation still passes because env supplies the required
values.

If step 5 fails after the rename, the server restores the previous file bytes
from memory and returns 500. A failed write never changes the running config.

## Runtime apply

PATCH and SIGHUP share one `applyConfig(Config)` path:

1. Publish the new `Config` (see Concurrency).
2. Call `KeyManager.ReloadKeys`. Existing key state (health, cooldown, usage)
   carries over for keys that remain. New keys start `unknown`, removed keys
   disappear. Strategy, session TTL, balanced idle timeout, cooldown, and
   proactive threshold all update there.
3. Restart the usage poller when `usage_check_interval` or
   `disable_usage_polling` differs from the previous config. SIGHUP now also
   picks up interval changes, which it previously ignored.
4. Rebuild the alert notifier and the workspace client only when their config
   sections changed. Dashboard edits never touch those sections.
5. Log one line with the changed field names and the config source. Never log
   values.

The usage poller becomes App-managed. `startUsagePoller` stores a
`context.CancelFunc`; apply cancels it and starts a new ticker only if the new
config still enables polling.

## Concurrency

`ReloadConfig` currently assigns `a.config`, `a.sender`, and `a.workspace` while
request handlers read them. Dashboard writes make that race wider, so the
design fixes it:

- `App.config` becomes `atomic.Pointer[Config]`. Each apply builds a fresh
  `Config` and publishes it. Handlers take one snapshot at entry through an
  accessor.
- `App.sender` and `App.workspace` become atomic pointers with snapshot
  accessors. `handleAdmin` takes one workspace snapshot per request.
- A `configApplyMu` mutex serializes PATCH and SIGHUP so two applies cannot
  interleave.
- `proxyV1` currently reads `a.keys.priorities` without the key manager lock.
  Add a `KeyPriority(index)` accessor.

## Dashboard UI

A new Proxy Configuration dialog, separate from the browser-local Dashboard
Settings dialog. The actions dropdown gets a `Proxy configuration` item.

Layout:

- Routing section: strategy select, `session_ttl`, `balanced_idle_timeout`,
  `retry_exhausted_after` as duration text inputs,
  `proactive_switch_threshold` as a 0 to 100 number input.
- Polling section: `usage_check_interval` and `disable_usage_polling`.
- Models section: `sanitize_developer_role` and alias rows with from, to, and
  delete.
- Upstream keys section: table of hint, priority, weight, rotate, and delete,
  plus an Add key row.
- Footer shows `config_source`. Env-locked fields render disabled with an env
  badge whose title names the variable. When `editable` is false the whole form
  is disabled with an explanation.

Behavior:

- Opening the dialog fetches GET. Form values come from that response.
- Save diffs the form against the loaded baseline, builds a PATCH body with
  only changed fields, and includes `if_revision`.
- Client-side checks cover duration format and numeric ranges. The server
  remains authoritative.
- 409 reloads the form and shows a message about a conflicting change. 400 and
  412 show the server message on the banner. Success shows a toast, re-renders
  from the response, and refreshes usage and metrics.
- Rotate inputs are empty by default. An empty rotate input keeps the existing
  secret. A new key row requires a value.
- Key list editing includes every existing key as `{id, priority, weight}` in
  the patch, plus `key` only when a rotation value was entered. Deleted rows
  are omitted. New rows have no id.

Files:

- `web/dashboard/src/components/ProxyConfig.ts` for rendering and
  form-to-patch conversion.
- `fetchProxyConfig` and `patchProxyConfig` in `api.ts`, types in `types.ts`.
- Dialog markup in `index.html`, styles in `style.css`, wiring in `main.ts`
  using the existing render function and event delegation patterns.

No frontend framework is added.

## Security

- Upstream key values never appear in a response. GET returns only hints.
- Key rotation writes the new value to the config file, which keeps `0600`.
- Logs list field names only.
- Auth is the existing proxy API key. There are no cookies, so CSRF is not a
  concern. The dashboard sends the bearer token from localStorage or the
  embedded loopback key.

## Testing

Go tests in `config_admin_test.go`:

- Patch merge for settings, null reset, alias delete, and key list edits
  (keep, rotate, add, remove).
- Rejections: unknown fields, duplicate ids, empty key list, invalid duration,
  stale revision, env-locked field.
- YAML writer preserves comments, key order, and unknown keys, and creates a
  file when none exists.
- GET masks keys and reports env-locked fields.
- PATCH applies runtime state: published config changes and key manager state
  matches the new key list.
- Poller restart is invoked when the interval or disable flag changes, using an
  injectable start function.
- Rollback when the post-write load fails.

Frontend: `npm run build` must pass (`tsc --noEmit` plus vite build). Manual
check in a browser against a server running with a temp config covering save,
env-locked fields, conflict handling, and key rotation.

## Docs

- `docs/admin-api.md`: new Configuration section with both endpoints, the
  payloads, and the status codes.
- `docs/configuration.md`: new section on dashboard editing, covering priority
  (dashboard writes the file, env still wins), env-locked fields, the created
  file when none exists, file permissions, and the partial-file caveat for
  env-only startup.

## Files touched

- `main.go`: atomic runtime state, shared apply path, restartable usage
  poller, `KeyPriority` accessor, two new routes in `handleAdmin`.
- `config_admin.go` (new): handlers, patch merge, revision, env locks, YAML
  node editing, atomic write.
- `config_admin_test.go` (new).
- `dashboard.go`: no functional change.
- `workspace.go`: call sites move to the config snapshot accessor.
- `main_test.go`: adjust existing reload tests if signatures change.
- Dashboard files listed above.
- Docs listed above.
