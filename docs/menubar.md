# macOS Menu Bar App

Native companion to the Switchboard Go proxy: live key/quota state in the
menu bar, notifications on exhaustion/recovery, and admin actions without
opening a browser.

## Build

    cd menu-bar
    swift build -c release

Binary: `menu-bar/.build/release/SwitchboardMenu`. Requires macOS 13+.

## Bundle as .app (recommended)

The raw binary dies with the terminal and can't use Launch at login.
Package it instead:

    cd menu-bar
    ./package-app.sh --install --open

This creates `SwitchboardMenu.app` (`LSUIElement`, ad-hoc signed, Finder
icon generated from `assets/favicon.svg` via `sips`/`iconutil`) and,
with `--install`, copies it to `/Applications` — required for
`SMAppService` Launch at login to work. Then enable
`Settings → Launch at login` in the popover. Use `--icon PATH` for a
custom icon source or `--no-icon` to skip icon generation.

## Run

    menu-bar/.build/release/SwitchboardMenu &

The app discovers your proxy the same way the Go binary does:
`SWITCHBOARD_GO_CONFIG`, then `./config.yaml` (relative to the app's working
directory), then `~/.config/switchboard-go/config.yaml`, then
`/etc/switchboard-go/config.yaml`. `PROXY_API_KEY` and `LISTEN_ADDR` env vars
override the file. If nothing is found it defaults to
`http://127.0.0.1:8495`; paste your proxy key in the popover → Settings.

A manually saved key is stored in the macOS Keychain and takes precedence
over config discovery.

## Features

- Menu bar label: rolling % + available/total keys, warning icon on trouble
- Popover: per-key masked hints, state, rolling/weekly/monthly bars, reset
  countdowns, exhaustion/switch counters
- Actions: Refresh, Validate keys, Reset all, Reload config
- Notifications: key exhausted / recovered / all exhausted
- Poll interval: 15s/30s/60s (Settings); Launch at login via SMAppService
