# Configuration

Switchboard Go supports YAML config files and environment variables.

Configuration precedence, from lowest to highest:

1. Built-in defaults
2. YAML config file
3. Environment variables

This lets you keep normal settings in a config file while injecting secrets with
environment variables, systemd environment files, Docker secrets, or your
deployment platform.

## Config file discovery

Switchboard Go looks for a config file in this order:

1. `SWITCHBOARD_GO_CONFIG`, if set
2. `./config.yaml` or `./config.yml` (current working directory), if it exists
3. `~/.config/switchboard-go/config.yaml`, if it exists
4. `/etc/switchboard-go/config.yaml`, if it exists
5. No config file

If `SWITCHBOARD_GO_CONFIG` is set, the file must exist and be valid YAML.
Missing or invalid explicit config files are startup errors.

Recommended user config path:

```text
~/.config/switchboard-go/config.yaml
```

Recommended system config path:

```text
/etc/switchboard-go/config.yaml
```

Use restrictive permissions, such as `0600`, for config files containing
secrets.

## YAML example

```yaml
server:
  listen_addr: "127.0.0.1:8080"
  proxy_api_key: "replace-with-a-long-random-local-key"

upstream:
  base_url: "https://opencode.ai/zen/go/v1"
  api_keys:
    # Plain key strings (defaults to priority: 1, weight: 1):
    - "sk-first"
    # Or structured key entries with priority tiers (1 = primary, 2+ = backup) and traffic weights:
    - key: "sk-primary-heavy"
      priority: 1
      weight: 3
    - key: "sk-backup"
      priority: 2
      weight: 1
  # Routing strategy: "session_sticky" (default), "balanced", "round_robin", "fill_first"
  routing_strategy: "session_sticky"
  # How long an idle session retains its assigned key before re-evaluating (default: 2h)
  session_ttl: "2h"
  # In balanced mode, max idle gap before switching to a key with lower quota (default: 1h)
  balanced_idle_timeout: "1h"
  # Upstream /usage polling interval for quota tracking and automatic recovery (default: 30s)
  usage_check_interval: "30s"
  # Rolling window percent threshold at which the proxy proactively rotates keys (default: 95.0)
  proactive_switch_threshold: 95.0
  # Disable background usage polling (default: false)
  disable_usage_polling: false
  # Fallback cooldown before an exhausted key is retried if resetsAt is unavailable (default: 5m)
  retry_exhausted_after: "5m"

# Model aliasing: seamlessly maps requested model IDs to upstream models
models:
  aliases:
    "gpt-4o": "glm-5.1"
    "gpt-4o-mini": "minimax-m3"
    "claude-3-7-sonnet": "glm-5.1"

# Request transformations
transformations:
  # Automatically convert OpenAI role "developer" to "system" for models that reject developer role (default: true)
  sanitize_developer_role: true

alerts:
  webhooks:
    - url: "https://discord.com/api/webhooks/..."
      type: "discord" # "generic", "discord", "slack"
    - url: "https://hooks.slack.com/services/..."
      type: "slack"
    - url: "https://example.com/alerts/switchboard"
      type: "generic"
  telegram:
    bot_token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
    chat_id: "987654321"

smtp:
  host: "smtp.example.com"
  port: 587
  username: "alerts@example.com"
  password: "your-smtp-password"
  from: "alerts@example.com"
  to: "you@example.com"
  tls: false
  starttls: true

limits:
  max_request_body_bytes: 20971520
```

## Key Priority & Weighting

- **Priority Tiers (`priority`)**: `1` (primary, default), `2` (backup), etc. Keys in higher priority tiers are exhausted or saturated before traffic falls back to lower priority backup tiers.
- **Traffic Weighting (`weight`)**: Relative weight for traffic distribution (default: `1`). Under `round_robin`, traffic is smoothly interleaved according to relative weights (e.g. `3:1` sends 75% of requests to weight-3 keys). Under `session_sticky`, higher weighted keys receive proportional preference when new sessions are assigned.

## Request Transformation & Compatibility

- **Automatic Developer Role Sanitization**: When enabled (`sanitize_developer_role: true`, default `true`), messages with `role: "developer"` in OpenAI chat completion payloads are rewritten to `role: "system"`.
- **Model Aliasing**: Configured mappings in `models.aliases` (or `MODEL_ALIASES` env) rewrite `"model"` in request payloads and augment `GET /v1/models` and `GET /models` responses.
- **Root Endpoints**: `/responses`, `/embeddings`, `/usage`, `/chat/completions`, `/messages`, and `/models` are routed seamlessly with or without the `/v1` prefix.

## Modern Alert Webhooks & Notifications

Switchboard Go supports multi-destination alerts on key rotation and key exhaustion events:
- **Discord**: Webhook formatted as rich markdown alert.
- **Slack**: Webhook formatted as standard incoming webhook message.
- **Telegram**: Bot API message sent to designated `chat_id`.
- **Generic HTTP POST**: JSON payload with event type, timestamp, key index, and complete status.
- **SMTP**: Email notifications via TLS/STARTTLS.

## Routing Strategies

- **`session_sticky`** (default): Routes all requests for a session/conversation to the same upstream key as long as it has capacity (< 95% rolling usage). New sessions are assigned to the subscription key with the lowest weekly/monthly usage. Retains KV prompt caches across back-and-forth chat turns. Sessions expire after 2 hours of inactivity (`session_ttl`).
- **`balanced`**: Requests made within rapid succession (idle gap < `balanced_idle_timeout`, default 1h) stick to the current key to preserve active prompt caches. When an idle gap occurs, Switchboard Go re-evaluates the key pool and routes subsequent requests to the key with the lowest weekly/monthly quota.
- **`round_robin`**: Rotates sequentially through eligible, non-saturated keys on every request.
- **`fill_first`**: Fills Key 0 up to the proactive switch threshold (default 95%) or exhaustion, then moves to Key 1, Key 2, etc.

## Workspace usage scraping

Per-model cost and quota breakdowns are scraped from the OpenCode console and
shown in the dashboard's workspace usage section. The feature is off when no
session cookie is configured.

### YAML

```yaml
workspace_usage:
  # Session cookie from opencode.ai (DevTools → Application → Cookies → "auth")
  session_cookie: ""
  # Optional: restrict to specific workspace IDs (default: auto-discover all)
  workspace_ids:
    - "wrk_01M1CDMTPY3W0AGGKXZA0147N5"
  # Polling interval (default: 60s, 0 disables polling)
  interval: "60s"

server:
  # Dashboard first-load proxy key: "auto" (default) embeds the key only when
  # listen_addr is loopback; "true" always; "false" never.
  dashboard_auto_key: "auto"
```

| Field | Default | Description |
| --- | --- | --- |
| `workspace_usage.session_cookie` | `""` (disabled) | `auth` cookie value from `https://opencode.ai`. When empty, scraping is disabled and `GET /admin/workspace-usage` returns `{"enabled": false}`. |
| `workspace_usage.workspace_ids` | auto-discover | Restrict scraping to specific workspace IDs. When empty, all workspaces visible to the cookie are scraped. |
| `workspace_usage.interval` | `60s` | Polling interval for console scraping. `0` disables background polling. Must be `>= 0`. |
| `server.dashboard_auto_key` | `"auto"` | Controls whether the dashboard HTML embeds the proxy key for first-load convenience. `auto` embeds only when `listen_addr` is loopback (`127.0.0.1`, `::1`, `localhost`); `true` always embeds; `false` never embeds. The Settings panel override in localStorage still takes precedence. |

### Environment variables

| Variable | Description |
| --- | --- |
| `OPENCODE_SESSION_COOKIE` | Overrides `workspace_usage.session_cookie`. |
| `OPENCODE_WORKSPACE_IDS` | Overrides `workspace_usage.workspace_ids` (comma-separated, e.g. `wrk_a,wrk_b`). |
| `WORKSPACE_USAGE_INTERVAL` | Overrides `workspace_usage.interval` (Go duration, e.g. `60s`). |
| `DASHBOARD_AUTO_KEY` | Overrides `server.dashboard_auto_key` (`auto` \| `true` \| `false`). |

### Obtaining the session cookie

1. Log in at `https://opencode.ai`.
2. Open DevTools → Application → Cookies → `https://opencode.ai`.
3. Copy the value of the `auth` cookie.
4. Paste it into `workspace_usage.session_cookie` or `OPENCODE_SESSION_COOKIE`.

The cookie is valid for up to 365 days. Treat it as a secret — restrict config file permissions to `0600` and prefer environment injection in shared environments.

### Failure behavior

Workspace usage scraping is best-effort and never affects proxying. If the cookie is missing, expired, or the console is unreachable, the poller records the error and `GET /admin/workspace-usage` returns the error in its snapshot; the dashboard shows an error banner in the workspace usage section. Proxying, quota polling, and all other endpoints continue to work normally.

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `SWITCHBOARD_GO_CONFIG` | No | | Explicit YAML config path. |
| `PROXY_API_KEY` | Yes\* | | API key clients must use to access this proxy. |
| `OPENCODE_GO_API_KEYS` | Yes\* | | Comma-separated OpenCode Go API keys. |
| `OPENCODE_GO_API_KEY_PRIORITIES` | No | `1` | Comma-separated priority tiers for each key (e.g. `1,1,2`). |
| `OPENCODE_GO_API_KEY_WEIGHTS` | No | `1` | Comma-separated weights for each key (e.g. `3,1,1`). |
| `LISTEN_ADDR` | No | `:8080` | HTTP listen address. Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenCode Go upstream base URL. |
| `ROUTING_STRATEGY` | No | `session_sticky` | Key selection strategy (`session_sticky`, `balanced`, `round_robin`, `fill_first`). |
| `SESSION_TTL` | No | `2h` | Inactivity duration after which a session mapping expires. |
| `BALANCED_IDLE_TIMEOUT` | No | `1h` | Idle gap duration before `balanced` strategy switches keys. |
| `USAGE_CHECK_INTERVAL` | No | `30s` | Polling frequency for upstream key quota telemetry. |
| `PROACTIVE_SWITCH_THRESHOLD` | No | `95.0` | Rolling usage percentage at which proxy proactively rotates away from a key. |
| `DISABLE_USAGE_POLLING` | No | `false` | Disable background usage polling. |
| `SANITIZE_DEVELOPER_ROLE` | No | `true` | Convert `developer` role to `system` in OpenAI chat payloads. |
| `MODEL_ALIASES` | No | | Comma-separated model aliases (`gpt-4o=glm-5.1,claude-3-7-sonnet=glm-5.1`). |
| `WEBHOOK_URL` / `GENERIC_WEBHOOK_URL` | No | | Generic HTTP POST webhook URL for alerts. |
| `DISCORD_WEBHOOK_URL` | No | | Discord webhook URL for alerts. |
| `SLACK_WEBHOOK_URL` | No | | Slack incoming webhook URL for alerts. |
| `TELEGRAM_BOT_TOKEN` | No | | Telegram bot token. |
| `TELEGRAM_CHAT_ID` | No | | Telegram chat ID for alerts. |
| `MAX_REQUEST_BODY_BYTES` | No | `20971520` | Maximum request body size. Requests above this return `413`. |
| `RETRY_EXHAUSTED_AFTER` | No | `5m` | Fallback cooldown before an exhausted key is retried. `0` disables cooldown. |
| `SMTP_HOST` | No | | SMTP host for notifications. |
| `SMTP_PORT` | No | `25` | SMTP port. |
| `SMTP_USERNAME` | No | | SMTP username. If empty, SMTP AUTH is skipped. |
| `SMTP_PASSWORD` | No | | SMTP password. |
| `SMTP_FROM` | No | | Sender email address. Required for notifications. |
| `SMTP_TO` | No | | Recipient email address. Required for notifications. |
| `SMTP_TLS` | No | `false` | Use implicit TLS when connecting to SMTP. |
| `SMTP_STARTTLS` | No | `false` | Use STARTTLS if the server advertises it. |

\* Required unless provided by YAML config.

Notifications are disabled unless `SMTP_HOST`, `SMTP_FROM`, and `SMTP_TO` are
all set.

## Validate config

Run a config-only check without starting the server:

```bash
switchboard-go validate-config
```

It loads config using normal precedence, validates required values, and prints a
safe summary.

Example env file for systemd:

```bash
PROXY_API_KEY=replace-with-a-long-random-local-key
OPENCODE_GO_API_KEYS=sk-first,sk-second,sk-third
ROUTING_STRATEGY=session_sticky
SESSION_TTL=2h
SMTP_PASSWORD=replace-me
```
