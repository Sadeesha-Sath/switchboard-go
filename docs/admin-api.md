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
  "note": "Remaining usage is unavailable from opencode-go API."
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

## Usage and quota visibility

OpenCode Go currently does not appear to expose a public API endpoint for
remaining quota or usage by API key. Switchboard Go therefore cannot show exact
remaining usage. It tracks inferred state based on upstream responses and
reports that through `/admin/status`.

If a public usage/quota endpoint becomes available, it can be added without
changing the client-facing OpenAI-compatible API.
