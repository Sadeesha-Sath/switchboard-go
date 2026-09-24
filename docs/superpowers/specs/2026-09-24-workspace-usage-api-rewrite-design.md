# Workspace usage API rewrite

Date: 2026-09-24
Status: approved for planning

## Problem

`workspace.go` reads per-model cost and quota data from the OpenCode console by
discovering SolidStart server function hashes in the deployed JS bundle and parsing
seroval responses with an `auth` session cookie. The console was redesigned: the root
site is now a marketing page, the console moved to `https://opencode.ai/console/`, the
session cookie is `__Host-console_session`, and the old server functions are gone. The
dashboard shows a scrape error and no usage data.

## Goals

- Restore per-workspace, per-model usage in the dashboard using the new console API.
- Authenticate with a service account API key instead of a browser session cookie.
- Keep the `GET /admin/workspace-usage` response schema and the dashboard UI unchanged.
- Keep the feature best-effort. Proxy traffic, quota polling, and every other endpoint
  keep working when the key is missing, expired, or the console is unreachable.

## Non-goals

- Reworking the dashboard layout, window tabs, or row columns.
- Using the documented v1 CSV export. The dashboard does not need record-level data,
  and its cost column is zero on Go plans.
- Supporting multiple workspaces per process. One service key is bound to one
  workspace.
- Runtime editing of workspace usage settings from the dashboard. They stay file and
  env only, like alerts and SMTP.
- Session cookie auth, hash discovery, and seroval parsing.

## Verified API

All calls send `Authorization: Bearer oc_sk_...` and `Accept: application/json`.
Verified live on 2026-09-24 with an `all` permission key on a Go-plan workspace. Both
endpoints are undocumented; they come from the deployed console bundle. See
`docs/research/2026-09-24-opencode-console-usage-api.md` for sources.

### GET /console/api/go/status

Returns the subscription and its quota meters.

```json
{
  "product": "go",
  "access": {
    "startsAt": "2026-08-26T15:52:58.000Z",
    "endsAt": "2026-09-26T15:52:58.000Z",
    "meters": {
      "fiveHour": {
        "startsAt": "2026-09-24T04:59:21.277Z",
        "resetsAt": "2026-09-24T09:59:21.277Z",
        "limitMicroCents": "1200000000",
        "usedMicroCents": "84289897"
      },
      "week": {
        "startsAt": "2026-09-21T00:00:00.000Z",
        "resetsAt": "2026-09-28T00:00:00.000Z",
        "limitMicroCents": "3000000000",
        "usedMicroCents": "1013236711"
      },
      "month": {
        "limitMicroCents": "6000000000",
        "usedMicroCents": "5084232386"
      }
    }
  }
}
```

Microcents per USD is 1e8. The month meter has no `startsAt` or `resetsAt`. All
numeric fields are decimal strings.

### GET /console/api/usage/models

Returns per-model aggregates for a window.

```
GET /console/api/usage/models?since=<ISO8601>&page=1&pageSize=100
```

`since` is an ISO 8601 UTC timestamp. `pageSize` is 1 to 100, default 10. The response:

```json
{
  "items": [
    {
      "model": "deepseek-v4.1-flash",
      "provider": "opencode-go",
      "totalRequests": "3040",
      "totalInputTokens": "13416209",
      "totalOutputTokens": "3008601",
      "totalCacheReadTokens": "329606784",
      "totalCacheWrite5mTokens": "0",
      "totalCacheWrite1hTokens": "0",
      "totalCostMicroCents": "546483151"
    }
  ],
  "pageInfo": { "page": 1, "pageSize": 10, "total": 4, "pageCount": 1 }
}
```

For the same `since` boundary, the sums from this endpoint match
`/console/api/usage/summary` and track the Go meters, so `totalCostMicroCents` is the
same cost concept as the meter values. The API no longer returns a model display name,
a multiplier, an estimated flag, or a separate quota cost.

## Design

### Client

`WorkspaceUsageClient` keeps its public shape:

- `NewWorkspaceUsageClient(baseURL, serviceAPIKey string) *WorkspaceUsageClient`
- `Enabled() bool`
- `Refresh(ctx context.Context) error`
- `Snapshot() WorkspaceUsageSnapshot`

The client is disabled when the key is empty, matching the current nil-client
behavior. `startWorkspaceUsagePoller` is unchanged: one refresh at startup, then one
per `workspace_usage.interval` (default 60s).

One refresh makes four calls: `go/status`, then `usage/models` once per window with
`since` set to that window's start. Requests follow pages while
`page < pageInfo.pageCount`, capped at 10 pages per window. A failed window fetch sets
the workspace error and skips the remaining windows, matching the current behavior.

### Mapping

The snapshot carries one workspace entry: id `opencode-go`, name `OpenCode Go`.

| Snapshot field | Source |
| --- | --- |
| `windows.rolling` | `meters.fiveHour` |
| `windows.weekly` | `meters.week` |
| `windows.monthly` | `meters.month` |
| `usage_usd` | `usedMicroCents` / 1e8 |
| `limit_usd` | `limitMicroCents` / 1e8 |
| `usage_percent` | used / limit * 100, rounded to 1 decimal; 0 when limit is 0 |
| `reset_in_sec` | `resetsAt` minus now, clamped at 0; monthly uses `access.endsAt`; 0 when absent |
| `status` | `"ok"` |
| rows query | `since` = meter `startsAt`; monthly uses `access.startsAt` |
| `rows[].model` | `model` |
| `rows[].name` | `model`, since the API returns no display name |
| `rows[].cost` | `totalCostMicroCents` / 1e8 |
| `rows[].quota_cost` | same value as `cost` |
| `rows[].multiplier` | null |
| `rows[].estimated` | false |
| `rows[].contribution_percent` | row cost / window row total * 100, rounded to 0.1 |

Rows sort by cost descending. A missing meter omits that window from the map, and the
dashboard shows its empty state. A `go/status` response without `access` is a
workspace error: `no active Go subscription for this service key`.

### Config

```yaml
workspace_usage:
  service_api_key: "oc_sk_..."
  interval: "60s"
```

- `WorkspaceUsageConfig` becomes `{ServiceAPIKey string, Interval time.Duration}`.
- `session_cookie` and `workspace_ids` are removed from the YAML struct, the merge
  logic, and the env overrides.
- A config file that still sets `session_cookie` loads with a warning log that the
  field is no longer supported. The feature stays off unless `service_api_key` is set.
- Env overrides: `OPENCODE_SERVICE_API_KEY` replaces `OPENCODE_SESSION_COOKIE` and
  `OPENCODE_WORKSPACE_IDS`. `WORKSPACE_USAGE_INTERVAL` stays.
- The startup log line reports whether a service key is configured.

### Errors

- 401 or 403 from any call: workspace error `service API key rejected (expired or
  revoked)`.
- `go/status` non-200, unreadable JSON, or missing `access`: workspace error naming the
  endpoint and status, or `no active Go subscription for this service key`.
- Window fetch failure: workspace error, remaining windows skipped.
- No retry inside `Refresh`. The poller tries again on the next tick.
- `/admin/workspace-usage` keeps returning `enabled`, `updated_at`, `error`, and the
  last snapshot. The dashboard error banner path does not change.

## Testing

- `workspace_test.go` against `httptest`: window mapping and reset math, row mapping
  (cost duplication, contribution rounding, sort order), pagination across pages,
  401 handling, malformed JSON, and a response without `access`.
- `main_test.go`: `service_api_key` parsing, env override, the legacy `session_cookie`
  warning path, and interval validation.
- The admin endpoint test uses new fixtures.
- `go test ./...` and `go vet ./...` pass. The frontend and menu-bar builds are not
  touched.

## Files

- Rewrite `workspace.go` and `workspace_test.go`.
- Delete `seroval.go` and `seroval_test.go`.
- Edit `main.go` (config struct, YAML, env, merge, validation, startup log, client
  construction), `main_test.go`, `config.example.yaml`, `docs/configuration.md`,
  `docs/admin-api.md`.
- Update the local `bin/config.yaml` (untracked) with a `service_api_key`.

## Caveats

- The two endpoints are undocumented and can change with a console deploy. The failure
  mode is a dashboard error banner, and the fix stays inside `workspace.go`.
- Window totals come from the meters; per-model rows come from usage records. Records
  only cover recent history, so row sums can be lower than the window total.
- `name` now equals the model id. Multipliers and estimated flags no longer exist.
- One key means one workspace. Users with several workspaces run several instances or
  wait for multi-key support.
