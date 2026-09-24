# Admin API

Admin endpoints require:

```text
Authorization: Bearer <PROXY_API_KEY>
```

Authentication failures on `/v1/*` and `/admin/*` return an OpenAI-style JSON
error envelope with HTTP 401.

## Status

`GET /admin/status` returns inferred key state and the active key index.

```bash
curl http://127.0.0.1:8080/admin/status \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "current_key_index": 1,
  "keys": [
    {
      "index": 0,
      "state": "exhausted",
      "last_429_time": "2026-06-19T11:48:29Z",
      "current": false,
      "eligible": false,
      "retry_after_seconds": 142
    },
    {
      "index": 1,
      "state": "available",
      "current": true,
      "eligible": true
    }
  ],
  "retry_exhausted_after_seconds": 300,
  "note": "unknown means the key has not yet been validated or used since startup; an exhausted key becomes eligible for an automatic retry once retry_exhausted_after_seconds has elapsed since last_429_time or resetsAt is reached."
}
```

Key states are inferred by this proxy:

- `unknown`: key has not yet been proven available or exhausted
- `available`: key is currently selected and not marked exhausted
- `exhausted`: key returned a quota/usage-exhausted `429`

Additional fields:

- `key_hint`: masked identifier for the key (first 7 and last 4 characters, e.g.
  `sk-5R6d…z28B`) so keys with identical priority/weight can be told apart.
  Short keys are fully masked (`…`). The full key is never returned.
- `eligible`: whether the key may be handed out on the next request. An
  exhausted key becomes eligible again once its cooldown elapses.
- `retry_after_seconds`: remaining cooldown for an exhausted key that is not yet
  eligible. Omitted for eligible keys and when the cooldown is disabled.
- `retry_exhausted_after_seconds`: the configured cooldown before an exhausted
  key is retried automatically. `0` means the cooldown is disabled.

An exhausted key is retried automatically once its cooldown elapses; the next
real request acts as the probe. See
[Operations and security](operations.md) for details. The all-keys-exhausted
`429` also carries a `Retry-After` header pointing at the next probe window.

## Aggregated Usage & Quota
 
`GET /usage`, `GET /v1/usage`, or `GET /admin/usage` returns aggregated usage across all configured upstream keys. It is shaped for dual compatibility:
 
1. **Top-Level OpenCode Compatibility**: Top-level `rolling`, `weekly`, and `monthly` objects match OpenCode `/zen/go/v1/usage` and standard dashboard widgets (such as `dsh-opencode-usage`, Waybar, Polybar, and VS Code extensions) with average utilization and earliest reset times.
2. **Multi-Subscription Telemetry**: The `summary` and `keys` objects provide full multi-key pool metrics, active session counts, and per-key breakdown.

```bash
curl http://127.0.0.1:8080/v1/usage \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

To force an immediate live poll of upstream usage before responding, add `?refresh=true`:

```bash
curl "http://127.0.0.1:8080/usage?refresh=true" \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "rolling": {
    "status": "ok",
    "percent": 45.0,
    "resetsAt": "2026-09-01T15:00:00Z"
  },
  "weekly": {
    "status": "ok",
    "percent": 25.0,
    "resetsAt": "2026-09-07T00:00:00Z"
  },
  "monthly": {
    "status": "ok",
    "percent": 12.5,
    "resetsAt": "2026-10-01T00:00:00Z"
  },
  "summary": {
    "total_keys": 2,
    "available_keys": 2,
    "exhausted_keys": 0,
    "active_sessions": 3,
    "routing_strategy": "session_sticky",
    "proactive_threshold_percent": 95.0,
    "pool_usage": {
      "rolling": {
        "average_percent": 45.0,
        "total_remaining_percent": 110.0,
        "min_percent": 30.0,
        "max_percent": 60.0,
        "earliest_reset_at": "2026-09-01T15:00:00Z"
      },
      "weekly": {
        "average_percent": 25.0,
        "total_remaining_percent": 150.0,
        "min_percent": 20.0,
        "max_percent": 30.0
      },
      "monthly": {
        "average_percent": 12.5,
        "total_remaining_percent": 175.0,
        "min_percent": 10.0,
        "max_percent": 15.0
      }
    }
  },
  "keys": [
    {
      "index": 0,
      "state": "available",
      "current": true,
      "eligible": true,
      "rolling": {"status": "ok", "percent": 30.0, "resetsAt": "2026-09-01T15:00:00Z"},
      "weekly": {"status": "ok", "percent": 20.0, "resetsAt": "2026-09-07T00:00:00Z"},
      "monthly": {"status": "ok", "percent": 10.0, "resetsAt": "2026-10-01T00:00:00Z"},
      "last_checked_at": "2026-09-01T14:30:00Z"
    },
    {
      "index": 1,
      "state": "available",
      "current": false,
      "eligible": true,
      "rolling": {"status": "ok", "percent": 60.0, "resetsAt": "2026-09-01T16:00:00Z"},
      "weekly": {"status": "ok", "percent": 30.0, "resetsAt": "2026-09-07T00:00:00Z"},
      "monthly": {"status": "ok", "percent": 15.0, "resetsAt": "2026-10-01T00:00:00Z"},
      "last_checked_at": "2026-09-01T14:30:00Z"
    }
  ]
}
```

## Workspace usage

`GET /admin/workspace-usage` returns the last per-model cost and quota breakdown read from the OpenCode console API. Requires `Authorization: Bearer <PROXY_API_KEY>`. When the feature is disabled (no `workspace_usage.service_api_key`), it returns `{"enabled": false, "workspaces": []}`.

```bash
curl http://127.0.0.1:8080/admin/workspace-usage \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "enabled": true,
  "updated_at": "2026-09-24T06:30:00Z",
  "workspaces": [
    {
      "id": "opencode-go",
      "name": "OpenCode Go",
      "windows": {
        "monthly": {
          "status": "ok",
          "usage_usd": 50.84232386,
          "limit_usd": 60.0,
          "usage_percent": 84.7,
          "reset_in_sec": 172800,
          "rows": [
            {
              "model": "deepseek-v4.1-flash",
              "name": "deepseek-v4.1-flash",
              "cost": 5.46483151,
              "quota_cost": 5.46483151,
              "contribution_percent": 100.0,
              "estimated": false
            }
          ]
        }
      }
    }
  ]
}
```

One service key is bound to one workspace. Each workspace has up to three windows (`rolling`, `weekly`, `monthly`), each with `status`, `usage_usd`, `limit_usd`, `usage_percent`, `reset_in_sec`, and per-model `rows` (`model`, `name`, `cost`, `quota_cost`, `contribution_percent`, `estimated`). Window totals come from the Go subscription meters; rows come from usage records, so row sums can be lower than the window total. Rows no longer carry a `multiplier`, and `name` equals the model id. On failure the `error` field is set at the top level or on the workspace entry, and the dashboard shows an error. See [Configuration](configuration.md#workspace-usage) for setup.

## Prometheus Metrics

`GET /metrics`, `GET /v1/metrics`, or `GET /admin/metrics` returns standard Prometheus text exposition metrics for scraping by Prometheus, Grafana Agent, or VictoriaMetrics.

```bash
curl http://127.0.0.1:8080/metrics
```

Example metrics output:

```text
# HELP switchboard_http_requests_total Total number of HTTP requests processed.
# TYPE switchboard_http_requests_total counter
switchboard_http_requests_total{endpoint="/v1/chat/completions",method="POST",status="200"} 42

# HELP switchboard_http_request_duration_seconds HTTP request latency distributions.
# TYPE switchboard_http_request_duration_seconds histogram
switchboard_http_request_duration_seconds_bucket{endpoint="/v1/chat/completions",method="POST",le="0.05"} 10
switchboard_http_request_duration_seconds_bucket{endpoint="/v1/chat/completions",method="POST",le="+Inf"} 42
switchboard_http_request_duration_seconds_sum{endpoint="/v1/chat/completions",method="POST"} 8.35
switchboard_http_request_duration_seconds_count{endpoint="/v1/chat/completions",method="POST"} 42

# HELP switchboard_upstream_requests_total Total number of upstream requests sent.
# TYPE switchboard_upstream_requests_total counter
switchboard_upstream_requests_total{key_index="0",priority="1",status="200"} 30
switchboard_upstream_requests_total{key_index="1",priority="2",status="200"} 12

# HELP switchboard_key_status State of upstream key (1 for active, 0 otherwise).
# TYPE switchboard_key_status gauge
switchboard_key_status{key_index="0",priority="1",state="available"} 1
switchboard_key_status{key_index="1",priority="2",state="available"} 1

# HELP switchboard_quota_usage_percent Key quota usage percentage.
# TYPE switchboard_quota_usage_percent gauge
switchboard_quota_usage_percent{key_index="0",window="rolling"} 30.00
switchboard_quota_usage_percent{key_index="0",window="weekly"} 20.00
switchboard_quota_usage_percent{key_index="0",window="monthly"} 10.00

# HELP switchboard_active_sessions Count of active in-memory sessions.
# TYPE switchboard_active_sessions gauge
switchboard_active_sessions 3
```

## Dashboard

A static web dashboard is embedded in the binary and served without
authentication:

- `GET /` redirects to `/dashboard/`
- `GET /dashboard/` serves the dashboard shell (HTML/JS/CSS)

The shell itself contains no sensitive data. Its JavaScript calls the
authenticated endpoints above using the proxy key entered in the dashboard's
Settings panel.

`GET /dashboard/api/metrics.json` returns the [Prometheus metrics](#prometheus-metrics)
contents as structured JSON (request counters, latencies, per-key upstream
traffic, exhaustions, switches, active sessions) plus `model_aliases` from the
configuration. Like `/metrics`, it is unauthenticated and contains no API keys.

## Configuration

`GET /admin/config` returns the editable settings, model aliases, and masked
upstream keys. `PATCH /admin/config` merges changes into the running config,
writes them to the config file, and applies them without a restart. Both
endpoints require the proxy API key.

```json
{
  "config_source": "/home/user/.config/switchboard-go/config.yaml",
  "editable": true,
  "revision": "6d1f…",
  "env_locked": ["routing_strategy"],
  "settings": {
    "routing_strategy": "session_sticky",
    "session_ttl": "2h",
    "balanced_idle_timeout": "1h",
    "proactive_switch_threshold": 95,
    "retry_exhausted_after": "5m",
    "usage_check_interval": "30s",
    "disable_usage_polling": false,
    "sanitize_developer_role": true
  },
  "model_aliases": { "gpt-4o": "glm-5.1" },
  "keys": [
    { "id": 0, "key_hint": "sk-abcd…1234", "priority": 1, "weight": 3 }
  ]
}
```

PATCH accepts only the fields being changed:

```json
{
  "if_revision": "6d1f…",
  "settings": { "routing_strategy": "balanced", "session_ttl": null },
  "model_aliases": { "gpt-4o": null, "claude-sonnet": "glm-5.1" },
  "keys": [
    { "id": 0, "priority": 2 },
    { "id": 1, "key": "sk-rotated-value" },
    { "key": "sk-new-key", "priority": 1, "weight": 2 }
  ]
}
```

- A `null` in `settings` resets that field to its default. A `null` in
  `model_aliases` deletes the alias.
- `keys`, when present, is the complete desired list. An entry with `id` edits
  that key. Omit `key` to keep the secret, or include it to rotate. An entry
  without `id` adds a key. Existing ids you leave out are removed.
- `if_revision` is the `revision` from GET. A stale value returns 409.
- Settings pinned by an env var (`env_locked`) return 409 when changed.
- Status codes: 400 invalid values, 409 stale revision or env lock, 412 no
  writable config file, 500 write or apply failure.

Values are written to `config_source`, preserving comments and keys the
dashboard does not manage. Environment variables still win over the file at
the next load.

## Reset key manually

`POST /admin/reset-key` un-marks an exhausted key immediately and makes it
eligible again. Useful after topping up a quota:

```bash
curl -X POST http://127.0.0.1:8080/admin/reset-key \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"index": 0}'
```

Returns the updated [`/admin/status`](#status) response.

## Reset all keys

`POST /admin/reset-all-keys` marks all keys available immediately:

```bash
curl -X POST http://127.0.0.1:8080/admin/reset-all-keys \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Returns the updated [`/admin/status`](#status) response.

## Validate keys against upstream

`POST /admin/validate-keys` sends a probe to upstream `GET /models` for every
configured key and returns each key's state:

```bash
curl -X POST http://127.0.0.1:8080/admin/validate-keys \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

## Reload configuration & keys

`POST /admin/reload` or sending a `SIGHUP` signal to the process dynamically reloads the configuration file, updating the active key pool, priorities, traffic weights, routing strategy, model aliases, and alert webhooks in-memory without downtime or dropping active sessions.

```bash
curl -X POST http://127.0.0.1:8080/admin/reload \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Or via POSIX signal:

```bash
kill -HUP $(pgrep switchboard-go)
```

Example response:

```json
{
  "status": "ok",
  "message": "configuration reloaded successfully",
  "total_keys": 3,
  "strategy": "session_sticky",
  "config_source": "/etc/switchboard-go/config.yaml"
}
```



## Health and readiness

`GET /healthz` is unauthenticated and intended for basic health checks.

`GET /readyz` is unauthenticated. It returns JSON, verifies required config is
loaded, and checks the currently selected non-exhausted upstream key by calling
`/models` with a 5 second timeout.
