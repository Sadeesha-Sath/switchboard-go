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

## Reset key manually

`POST /admin/reset-key` un-exhausts a single upstream key by index, without restarting the proxy. Body: `{"index": <int>}`. Responds with the updated status.

`POST /admin/reset-all-keys` un-exhausts all upstream keys immediately. Responds with the updated status.

## Validate keys

`POST /admin/validate-keys` actively checks every configured upstream key against
`/models`, updates in-memory key state, and returns per-key validation results.

```bash
curl -X POST http://127.0.0.1:8080/admin/validate-keys \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

## Health and readiness

`GET /healthz` is unauthenticated and intended for basic health checks.

`GET /readyz` is unauthenticated. It returns JSON, verifies required config is
loaded, and checks the currently selected non-exhausted upstream key by calling
`/models` with a 5 second timeout.
