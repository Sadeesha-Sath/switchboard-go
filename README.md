<p align="center">
  <img src="assets/logo.png" alt="Switchboard Go logo" width="220">
</p>

<h1 align="center">Switchboard Go</h1>

Switchboard Go is a small local proxy for the OpenCode Go API.

It gives OpenAI-compatible and Anthropic Messages-compatible tools one stable
local endpoint and automatically cycles through your upstream OpenCode Go API
keys when one is exhausted.

Most users should run it on their own computer:

```text
OpenAI/Anthropic-compatible app -> http://127.0.0.1:8080/v1 -> OpenCode Go
```

## Why use it?

- One local `/v1/*` endpoint for OpenAI-compatible and Anthropic Messages
  requests
- One proxy API key for your tools
- Multiple upstream OpenCode Go keys behind the scenes
- **Key Priority Tiers & Traffic Weighting**: Primary vs backup key tiers and smooth weighted traffic distribution
- **Request Transformation & Compatibility**: Automatic `developer` $\rightarrow$ `system` role sanitization, model name aliasing, and root endpoint support (`/responses`, `/embeddings`, `/usage`)
- **Modern Alert Webhooks**: Instant alerts to Discord, Slack, Telegram, generic JSON webhooks, and SMTP
- **Prometheus Metrics (`/metrics`)**: Production-ready exposition endpoint tracking requests, durations, key statuses, switches, 429s, and quota usage
- **Dynamic Configuration Reloading (`/admin/reload`, `SIGHUP`)**: Hot-reload keys, priorities, weights, aliases, and webhooks in-memory without downtime
- **Aggregated Quota Endpoint (`/usage`, `/v1/usage`)**: Compatible with OpenCode widget tools (Waybar, Polybar, VS Code) while exposing full pool metrics
- **Built-in Web Dashboard (`/dashboard`)**: Local UI for per-key quota, pool usage, live metrics, and admin actions (validate, reset, reload)
- **Multi-Strategy Routing**: `session_sticky` (default) preserves upstream KV prompt caching across agent turns; `balanced`, `round_robin`, and `fill_first` also available
- **Proactive Quota Switching**: Automatically switches away from subscriptions at $\ge 95\%$ capacity before hitting 429 errors
- **Dynamic Reset Cooldown & Recovery**: Keys cool down until their exact upstream `resetsAt` and automatically recover when quota resets
- Automatic failover when an upstream key is exhausted

## Install

Download a binary from GitHub Releases:

```text
https://github.com/ArsalanDotMe/switchboard-go/releases
```

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"
export LISTEN_ADDR="127.0.0.1:8080"

switchboard-go
```

## Use it from an OpenAI-compatible client

Use the proxy key, not your upstream OpenCode Go key:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

Example request:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 100
  }'
```

## Use it from an Anthropic Messages-compatible client

Anthropic-style clients should use the same base URL and proxy key. Switchboard
Go authenticates clients with the proxy key, then forwards upstream with the
current OpenCode Go key in `x-api-key`:

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
export ANTHROPIC_API_KEY="$PROXY_API_KEY"
```

Example request:

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: $PROXY_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "Say hello"}]
  }'
```

For opencode and Pi Coding Agent examples, see
[docs/agent-config.md](docs/agent-config.md).

## Common settings

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `PROXY_API_KEY` | Yes | | Key clients use to access Switchboard Go. |
| `OPENCODE_GO_API_KEYS` | Yes | | Comma-separated upstream OpenCode Go API keys. |
| `LISTEN_ADDR` | No | `:8080` | Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenCode Go upstream base URL. |
| `ROUTING_STRATEGY` | No | `session_sticky` | Strategy (`session_sticky`, `balanced`, `round_robin`, `fill_first`). |
| `SESSION_TTL` | No | `2h` | Inactivity TTL for session stickiness. |
| `BALANCED_IDLE_TIMEOUT` | No | `1h` | Idle gap timeout before `balanced` strategy switches keys. |
| `USAGE_CHECK_INTERVAL` | No | `30s` | Polling frequency for upstream key quota telemetry. |
| `PROACTIVE_SWITCH_THRESHOLD` | No | `95.0` | Rolling usage % at which proxy proactively rotates keys. |
| `RETRY_EXHAUSTED_AFTER` | No | `5m` | Fallback cooldown before an exhausted key is retried. `0` disables it. |

YAML config is also supported. See
[docs/configuration.md](docs/configuration.md).

## Web dashboard

A built-in dashboard is served at:

```text
http://127.0.0.1:8080/dashboard/
```

It shows per-key quota windows (rolling/weekly/monthly), pool aggregates, key
states and cooldowns, live request/latency metrics, and the admin actions
(validate keys, reset keys, reload config). Model aliases from your config are
listed in the footer.

The dashboard shell loads without authentication. Data calls use your
`PROXY_API_KEY`, entered once in the dashboard's Settings panel and stored in
the browser's localStorage; every request is sent as
`Authorization: Bearer $PROXY_API_KEY`.

## Usage and admin endpoints

Use `Authorization: Bearer $PROXY_API_KEY`:

- `GET /usage` or `GET /v1/usage` (aggregated quota, supports `?refresh=true`)
- `GET /admin/status` (in-memory key states and cooldown status)
- `POST /admin/validate-keys` (active probe of all keys against `/models`)
- `POST /admin/reset-key` (un-exhaust a specific key by index)
- `POST /admin/reset-all-keys` (un-exhaust all keys immediately)

Health checks:

- `GET /healthz`
- `GET /readyz`

See [docs/admin-api.md](docs/admin-api.md).

## More docs

- [Configuration](docs/configuration.md)
- [Agent/client setup](docs/agent-config.md)
- [Admin API](docs/admin-api.md)
- [Docker](docs/docker.md)
- [systemd deployment](docs/deployment.md)
- [SMTP notifications](docs/smtp.md)
- [Operations and security](docs/operations.md)

## Development

```bash
go test ./...
gofmt -w .
go build ./...
```

## License

MIT. See [LICENSE](LICENSE).
