# macOS Menu Bar App (Phase 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A native macOS menu bar app that shows Switchboard Go's live key/quota state, fires notifications on key exhaustion/recovery, and exposes admin actions — mirroring the web dashboard without a browser.

**Architecture:** SwiftUI `MenuBarExtra` (`.window` style) driven by a `StatusModel: ObservableObject` that polls the proxy's `/usage` and `/dashboard/api/metrics.json` endpoints every 30s via `URLSession` with `Authorization: Bearer <key>`. Config is auto-discovered by replicating the Go binary's discovery order (`SWITCHBOARD_GO_CONFIG` → `./config.yaml` → `~/.config/switchboard-go/config.yaml` → `/etc/switchboard-go/config.yaml`, with `PROXY_API_KEY`/`LISTEN_ADDR` env overrides); a manually pasted key is stored in the Keychain and takes precedence. All display data is server-derived (including masked `key_hint` values) — the app never stores or logs full upstream keys.

**Tech Stack:** Swift 5.9 tools / Swift 5 language mode, SwiftUI + AppKit, SPM executable (no `.xcodeproj`), Yams (YAML parsing), XCTest, UserNotifications, SMAppService (login item). Minimum macOS 13 (Ventura).

**Spec:** Approved Phase 2 design in conversation (v2 plan, "Phase 2 — macOS Menu Bar (delta)"): NSStatusItem-style item showing available/total + rolling %, popover with summary + per-key list, 30s polling (matches server `usage_check_interval: 30s`), Bearer auth with key in Keychain, local notifications on `available→exhausted` and all-exhausted, actions (validate/reset/reload), "Open Dashboard" button, config-file auto-fill.

**Spec of the data it consumes:** the proxy endpoints documented in `docs/admin-api.md` (`GET /usage`, `GET /admin/status`, `GET /dashboard/api/metrics.json`, `POST /admin/validate-keys|reset-key|reset-all-keys|reload`). Example JSON payloads in that doc are the fixtures used by tests.

## Global Constraints

- Minimum deployment target: **macOS 13.0** (`MenuBarExtra`, `SMAppService`).
- `swift-tools-version:5.9`, **Swift 5 language mode** (avoid Swift 6 strict-concurrency friction in `@main App` init).
- Only third-party dependency: **Yams** (`https://github.com/jpsim/Yams.git`, from 5.1.0). No HTTP libs, no YAML hand-parsing.
- Executable product name: **`SwitchboardMenu`**; package dir: **`menu-bar/`** at repo root.
- Endpoints (all under proxy base URL, default `http://127.0.0.1:8495`): `/usage`, `/usage?refresh=true`, `/admin/status`, `/dashboard/api/metrics.json`, `POST /admin/validate-keys`, `POST /admin/reset-key` (JSON body `{"index": n}`), `POST /admin/reset-all-keys`, `POST /admin/reload`.
- Auth header on `/usage`, `/admin/*`: `Authorization: Bearer <PROXY_API_KEY>`. `/dashboard/api/metrics.json` and `/metrics` are unauthenticated (sent key harmless).
- The **proxy key is a secret**: never printed to logs, never embedded in notifications, never shown unmasked in the UI (Settings field is a `SecureField`).
- Add `menu-bar/.build/` to the repo root `.gitignore` (Task 1).
- Every commit: conventional-commit message + `git commit -s` (repo AGENTS.md requires signoff).
- Work at execution time in worktree `./.worktrees/menu-bar` created via the `superpowers:using-git-worktrees` skill (AGENTS.md: worktrees live in `./.worktrees/`).
- Verification binaries are killed with **exact-name** match only: `pkill -x SwitchboardMenu` (never broad `pkill -f` patterns — a broad pattern killed the user's running proxy earlier in this project).

---

### Task 1: SPM package scaffold with a placeholder menu bar item

**Files:**
- Create: `menu-bar/Package.swift`
- Create: `menu-bar/Sources/SwitchboardMenu/SwitchboardMenuApp.swift`
- Create: `menu-bar/Sources/SwitchboardMenu/StatusModel.swift` (stub, so Task 6 modifies rather than creates)
- Modify: `.gitignore` (append `menu-bar/.build/`)

**Interfaces:**
- Produces: buildable package `SwitchboardMenu`; placeholder `SwitchboardMenuApp` that Task 7 replaces; `StatusModel` stub class `@MainActor final class StatusModel: ObservableObject` with empty body (Task 6 fills it).

- [ ] **Step 1: Write Package.swift**

```swift
// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SwitchboardMenu",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SwitchboardMenu", targets: ["SwitchboardMenu"])
    ],
    targets: [
        .executableTarget(name: "SwitchboardMenu"),
        .testTarget(name: "SwitchboardMenuTests", dependencies: ["SwitchboardMenu"]),
    ]
)
```

(Yams dependency is added in Task 3, when the YAML parser needs it.)

- [ ] **Step 2: Write placeholder app entry**

```swift
import SwiftUI

@main
struct SwitchboardMenuApp: App {
    var body: some Scene {
        MenuBarExtra("Switchboard", systemImage: "arrow.triangle.swap") {
            Text("Switchboard Go — setup in progress")
        }
    }
}
```

- [ ] **Step 3: Write StatusModel stub**

```swift
import Foundation
import Combine

@MainActor
final class StatusModel: ObservableObject {}
```

- [ ] **Step 4: Ignore build artifacts**

Append to repo root `.gitignore`:

```text
menu-bar/.build/
```

- [ ] **Step 5: Verify build**

Run: `cd menu-bar && swift build`
Expected: `Build complete!` with no warnings.

- [ ] **Step 6: Commit**

```bash
git add .gitignore menu-bar
git commit -s -m "feat(menubar): scaffold SwiftPM package with placeholder menu bar item"
```

---

### Task 2: Codable models mirroring the proxy JSON

**Files:**
- Create: `menu-bar/Sources/SwitchboardMenu/Models.swift`
- Create: `menu-bar/Tests/SwitchboardMenuTests/ModelsTests.swift`

**Interfaces:**
- Produces (used by every later task): `AggregatedUsage`, `UsageWindow`, `UsageSummary`, `UsageSummaryPool`, `SummaryWindow`, `PerKeyUsage`, `StatusResponse`, `PerKeyStatus`, `MetricsSnapshot`, `HTTPRequestMetric`, `HTTPDurationMetric`, `UpstreamRequestMetric`, `KeyExhaustionMetric`, `KeySwitchMetric` — all `Codable, Equatable, Sendable`. Property names camelCase via `CodingKeys` matching the Go JSON tags exactly (`average_percent`, `key_hint`, `resetsAt`, `retry_after_seconds`, `model_aliases`, …).
- Field names mirror `main.go` structs: `AggregatedUsageResponse` (main.go:1657), `UsageSummary` (main.go:1632), `PerKeyUsage` (main.go:1642), `StatusResponse` (main.go:1611), `MetricsSnapshot` (dashboard.go).

- [ ] **Step 1: Write the failing decode tests**

```swift
import XCTest
@testable import SwitchboardMenu

final class ModelsTests: XCTestCase {
    // Fixture mirrors docs/admin-api.md "Aggregated Usage" example, plus key_hint.
    private let usageJSON = """
    {
      "rolling": {"status":"ok","percent":45.0,"resetsAt":"2026-09-01T15:00:00Z"},
      "weekly": {"status":"ok","percent":25.0,"resetsAt":"2026-09-07T00:00:00Z"},
      "monthly": {"status":"ok","percent":12.5},
      "summary": {
        "total_keys":2,"available_keys":2,"exhausted_keys":0,"active_sessions":3,
        "routing_strategy":"session_sticky","proactive_threshold_percent":95.0,
        "pool_usage": {
          "rolling": {"average_percent":45.0,"total_remaining_percent":110.0,"min_percent":30.0,"max_percent":60.0,"earliest_reset_at":"2026-09-01T15:00:00Z"},
          "weekly": {"average_percent":25.0,"total_remaining_percent":150.0,"min_percent":20.0,"max_percent":30.0},
          "monthly": {"average_percent":12.5,"total_remaining_percent":175.0,"min_percent":10.0,"max_percent":15.0}
        }
      },
      "keys": [
        {"index":0,"key_hint":"sk-5R6d…z28B","state":"available","priority":1,"weight":1,
         "current":true,"eligible":true,
         "rolling":{"status":"ok","percent":30.0,"resetsAt":"2026-09-01T15:00:00Z"},
         "weekly":{"status":"ok","percent":20.0},
         "monthly":{"status":"ok","percent":10.0},
         "last_checked_at":"2026-09-01T14:30:00Z"},
        {"index":1,"key_hint":"sk-DLgH…0oPw","state":"exhausted","priority":1,"weight":1,
         "current":false,"eligible":false,"retry_after_seconds":142,
         "rolling":{"status":"exhausted","percent":99.0},
         "weekly":{"status":"ok","percent":30.0},
         "monthly":{"status":"ok","percent":15.0},
         "error":"upstream /usage returned status 429"}
      ]
    }
    """

    func testDecodeAggregatedUsage() throws {
        let usage = try JSONDecoder().decode(AggregatedUsage.self, from: Data(usageJSON.utf8))
        XCTAssertEqual(usage.rolling.percent, 45.0)
        XCTAssertEqual(usage.rolling.resetsAt, "2026-09-01T15:00:00Z")
        XCTAssertEqual(usage.summary.totalKeys, 2)
        XCTAssertEqual(usage.summary.activeSessions, 3)
        XCTAssertEqual(usage.summary.routingStrategy, "session_sticky")
        XCTAssertEqual(usage.summary.proactiveThresholdPercent, 95.0)
        XCTAssertEqual(usage.summary.poolUsage?.rolling.totalRemainingPercent, 110.0)
        XCTAssertEqual(usage.keys.count, 2)
        XCTAssertEqual(usage.keys[0].keyHint, "sk-5R6d…z28B")
        XCTAssertTrue(usage.keys[0].current)
        XCTAssertEqual(usage.keys[1].state, "exhausted")
        XCTAssertEqual(usage.keys[1].retryAfterSeconds, 142)
        XCTAssertEqual(usage.keys[1].error, "upstream /usage returned status 429")
    }

    func testDecodeStatusResponse() throws {
        let json = """
        {"current_key_index":1,
         "keys":[{"index":0,"key_hint":"sk-5R6d…z28B","state":"exhausted","priority":1,
                  "last_429_time":"2026-06-19T11:48:29Z","current":false,"eligible":false,
                  "retry_after_seconds":142},
                 {"index":1,"state":"available","current":true,"eligible":true}],
         "retry_exhausted_after_seconds":300,
         "note":"…"}
        """
        let st = try JSONDecoder().decode(StatusResponse.self, from: Data(json.utf8))
        XCTAssertEqual(st.currentKeyIndex, 1)
        XCTAssertEqual(st.keys[0].last429Time, "2026-06-19T11:48:29Z")
        XCTAssertEqual(st.retryExhaustedAfterSeconds, 300)
    }

    func testDecodeMetricsSnapshot() throws {
        let json = """
        {"generated_at":"2026-09-01T18:00:29Z",
         "http_requests":[{"endpoint":"/usage","method":"GET","status":200,"count":10}],
         "http_durations":[{"endpoint":"/usage","method":"GET","duration_seconds_sum":0.25,"duration_seconds_count":10}],
         "upstream_requests":[{"key_index":0,"priority":1,"status":200,"count":30,"duration_seconds_sum":4.5,"duration_seconds_count":30}],
         "key_exhaustions":[{"key_index":1,"count":2}],
         "key_switches":[{"from_key":0,"to_key":1,"reason":"quota_429","count":1}],
         "active_sessions":3,
         "model_aliases":{"gpt-4o":"glm-5.1","gpt-4o-mini":"minimax-m3"}}
        """
        let snap = try JSONDecoder().decode(MetricsSnapshot.self, from: Data(json.utf8))
        XCTAssertEqual(snap.httpRequests.first?.endpoint, "/usage")
        XCTAssertEqual(snap.upstreamRequests.first?.count, 30)
        XCTAssertEqual(snap.keyExhaustions.first?.count, 2)
        XCTAssertEqual(snap.modelAliases?["gpt-4o"], "glm-5.1")
        XCTAssertEqual(snap.activeSessions, 3)
    }

    func testMissingOptionalsDecodeAsNil() throws {
        let json = """
        {"status":"ok","percent":0}
        """
        let win = try JSONDecoder().decode(UsageWindow.self, from: Data(json.utf8))
        XCTAssertNil(win.resetsAt)
        XCTAssertEqual(win.status, "ok")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd menu-bar && swift test --filter ModelsTests`
Expected: compile error — `AggregatedUsage` (and siblings) undefined.

- [ ] **Step 3: Write Models.swift**

```swift
import Foundation

public struct UsageWindow: Codable, Equatable, Sendable {
    public var status: String
    public var percent: Double
    public var resetsAt: String? = nil
}

public struct SummaryWindow: Codable, Equatable, Sendable {
    public var averagePercent: Double
    public var totalRemainingPercent: Double
    public var minPercent: Double
    public var maxPercent: Double
    public var earliestResetAt: String? = nil

    private enum CodingKeys: String, CodingKey {
        case averagePercent = "average_percent"
        case totalRemainingPercent = "total_remaining_percent"
        case minPercent = "min_percent"
        case maxPercent = "max_percent"
        case earliestResetAt = "earliest_reset_at"
    }
}

public struct UsageSummaryPool: Codable, Equatable, Sendable {
    public var rolling: SummaryWindow
    public var weekly: SummaryWindow
    public var monthly: SummaryWindow
}

public struct UsageSummary: Codable, Equatable, Sendable {
    public var totalKeys: Int
    public var availableKeys: Int
    public var exhaustedKeys: Int
    public var activeSessions: Int
    public var routingStrategy: String
    public var proactiveThresholdPercent: Double
    public var poolUsage: UsageSummaryPool? = nil

    private enum CodingKeys: String, CodingKey {
        case totalKeys = "total_keys"
        case availableKeys = "available_keys"
        case exhaustedKeys = "exhausted_keys"
        case activeSessions = "active_sessions"
        case routingStrategy = "routing_strategy"
        case proactiveThresholdPercent = "proactive_threshold_percent"
        case poolUsage = "pool_usage"
    }
}

public struct PerKeyUsage: Codable, Equatable, Sendable, Identifiable {
    public var index: Int
    public var keyHint: String? = nil
    public var state: String
    public var priority: Int
    public var weight: Int
    public var current: Bool
    public var eligible: Bool
    public var retryAfterSeconds: Int? = nil
    public var rolling: UsageWindow
    public var weekly: UsageWindow
    public var monthly: UsageWindow
    public var lastCheckedAt: String? = nil
    public var error: String? = nil

    public var id: Int { index }

    private enum CodingKeys: String, CodingKey {
        case index
        case keyHint = "key_hint"
        case state
        case priority
        case weight
        case current
        case eligible
        case retryAfterSeconds = "retry_after_seconds"
        case rolling, weekly, monthly
        case lastCheckedAt = "last_checked_at"
        case error
    }
}

public struct AggregatedUsage: Codable, Equatable, Sendable {
    public var rolling: UsageWindow
    public var weekly: UsageWindow
    public var monthly: UsageWindow
    public var summary: UsageSummary
    public var keys: [PerKeyUsage]
}

public struct PerKeyStatus: Codable, Equatable, Sendable {
    public var index: Int
    public var keyHint: String? = nil
    public var state: String
    public var priority: Int
    public var weight: Int
    public var last429Time: String? = nil
    public var current: Bool
    public var eligible: Bool
    public var retryAfterSeconds: Int? = nil

    private enum CodingKeys: String, CodingKey {
        case index
        case keyHint = "key_hint"
        case state, priority, weight
        case last429Time = "last_429_time"
        case current, eligible
        case retryAfterSeconds = "retry_after_seconds"
    }
}

public struct StatusResponse: Codable, Equatable, Sendable {
    public var currentKeyIndex: Int
    public var keys: [PerKeyStatus]
    public var retryExhaustedAfterSeconds: Int
    public var note: String

    private enum CodingKeys: String, CodingKey {
        case currentKeyIndex = "current_key_index"
        case keys
        case retryExhaustedAfterSeconds = "retry_exhausted_after_seconds"
        case note
    }
}

public struct HTTPRequestMetric: Codable, Equatable, Sendable {
    public var endpoint: String
    public var method: String
    public var status: Int
    public var count: Int
}

public struct HTTPDurationMetric: Codable, Equatable, Sendable {
    public var endpoint: String
    public var method: String
    public var durationSecondsSum: Double
    public var durationSecondsCount: Int

    private enum CodingKeys: String, CodingKey {
        case endpoint, method
        case durationSecondsSum = "duration_seconds_sum"
        case durationSecondsCount = "duration_seconds_count"
    }
}

public struct UpstreamRequestMetric: Codable, Equatable, Sendable {
    public var keyIndex: Int
    public var priority: Int
    public var status: Int
    public var count: Int
    public var durationSecondsSum: Double
    public var durationSecondsCount: Int

    private enum CodingKeys: String, CodingKey {
        case keyIndex = "key_index"
        case priority, status, count
        case durationSecondsSum = "duration_seconds_sum"
        case durationSecondsCount = "duration_seconds_count"
    }
}

public struct KeyExhaustionMetric: Codable, Equatable, Sendable {
    public var keyIndex: Int
    public var count: Int

    private enum CodingKeys: String, CodingKey {
        case keyIndex = "key_index"
        case count
    }
}

public struct KeySwitchMetric: Codable, Equatable, Sendable {
    public var fromKey: Int
    public var toKey: Int
    public var reason: String
    public var count: Int

    private enum CodingKeys: String, CodingKey {
        case fromKey = "from_key"
        case toKey = "to_key"
        case reason
        case count
    }
}

public struct MetricsSnapshot: Codable, Equatable, Sendable {
    public var generatedAt: String
    public var httpRequests: [HTTPRequestMetric]
    public var httpDurations: [HTTPDurationMetric]
    public var upstreamRequests: [UpstreamRequestMetric]
    public var keyExhaustions: [KeyExhaustionMetric]
    public var keySwitches: [KeySwitchMetric]
    public var activeSessions: Int
    public var modelAliases: [String: String]? = nil

    private enum CodingKeys: String, CodingKey {
        case generatedAt = "generated_at"
        case httpRequests = "http_requests"
        case httpDurations = "http_durations"
        case upstreamRequests = "upstream_requests"
        case keyExhaustions = "key_exhaustions"
        case keySwitches = "key_switches"
        case activeSessions = "active_sessions"
        case modelAliases = "model_aliases"
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd menu-bar && swift test --filter ModelsTests`
Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu/Models.swift menu-bar/Tests/SwitchboardMenuTests/ModelsTests.swift
git commit -s -m "feat(menubar): Codable models for /usage, /admin/status, and metrics snapshot"
```

---

### Task 3: Config discovery + YAML parsing (Yams)

**Files:**
- Modify: `menu-bar/Package.swift` (add Yams dependency)
- Create: `menu-bar/Sources/SwitchboardMenu/ConfigLocator.swift`
- Create: `menu-bar/Tests/SwitchboardMenuTests/ConfigLocatorTests.swift`

**Interfaces:**
- Consumes: nothing (Foundation + Yams only).
- Produces:
  - `struct ProxyConfig: Equatable { var baseURL: URL; var proxyAPIKey: String; var configSource: String }`
  - `enum ConfigLocator` with
    - `static func locate(cwd: String = FileManager.default.currentDirectoryPath, home: String = NSHomeDirectory(), env: [String: String] = ProcessInfo.processInfo.environment) -> ProxyConfig?`
    - `static func parse(yaml: String) -> ProxyConfig?`
    - `static func normalize(_ listenAddr: String) -> URL`
    - `static func applyEnvOverrides(_ cfg: ProxyConfig?, env: [String: String]) -> ProxyConfig?`
  - Discovery order matches Go `resolveConfigPath()` (main.go:157): `SWITCHBOARD_GO_CONFIG` → `<cwd>/config.yaml` → `<cwd>/config.yml` → `<home>/.config/switchboard-go/config.yaml` → `/etc/switchboard-go/config.yaml`. Env overrides match Go `applyEnvOverrides()` (main.go:441): `PROXY_API_KEY` replaces the key, `LISTEN_ADDR` replaces the base URL.

- [ ] **Step 1: Add Yams to Package.swift**

Replace the `package` body dependencies/targets so the file reads:

```swift
// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SwitchboardMenu",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "SwitchboardMenu", targets: ["SwitchboardMenu"])
    ],
    dependencies: [
        .package(url: "https://github.com/jpsim/Yams.git", from: "5.1.0"),
    ],
    targets: [
        .executableTarget(name: "SwitchboardMenu", dependencies: ["Yams"]),
        .testTarget(name: "SwitchboardMenuTests", dependencies: ["SwitchboardMenu"]),
    ]
)
```

- [ ] **Step 2: Write the failing tests**

```swift
import XCTest
@testable import SwitchboardMenu

final class ConfigLocatorTests: XCTestCase {
    private let sampleYAML = """
    server:
      listen_addr: "127.0.0.1:8495"
      proxy_api_key: "test-proxy-key-123"

    upstream:
      base_url: "https://opencode.ai/zen/go/v1"
      api_keys:
        - key: "sk-upstream-1"
          priority: 1
          weight: 1
    """

    func testParseExtractsServerSection() {
        let cfg = ConfigLocator.parse(yaml: sampleYAML)
        XCTAssertNotNil(cfg)
        XCTAssertEqual(cfg?.baseURL.absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
    }

    func testParseRejectsConfigWithoutProxyKey() {
        let yaml = "server:\n  listen_addr: \"127.0.0.1:8495\"\n"
        XCTAssertNil(ConfigLocator.parse(yaml: yaml))
    }

    func testParseRejectsInvalidYAML() {
        XCTAssertNil(ConfigLocator.parse(yaml: "::::not yaml::::"))
    }

    func testNormalize() {
        XCTAssertEqual(ConfigLocator.normalize(":8495").absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(ConfigLocator.normalize("0.0.0.0:8495").absoluteString, "http://127.0.0.1:8495")
        XCTAssertEqual(ConfigLocator.normalize("127.0.0.1:8495").absoluteString, "http://127.0.0.1:8495")
    }

    func testEnvOverrides() {
        var cfg = ConfigLocator.parse(yaml: sampleYAML)
        cfg = ConfigLocator.applyEnvOverrides(cfg, env: ["PROXY_API_KEY": "env-key", "LISTEN_ADDR": "127.0.0.1:9000"])
        XCTAssertEqual(cfg?.proxyAPIKey, "env-key")
        XCTAssertEqual(cfg?.baseURL.absoluteString, "http://127.0.0.1:9000")
    }

    func testEmptyEnvDoesNotOverride() {
        var cfg = ConfigLocator.parse(yaml: sampleYAML)
        cfg = ConfigLocator.applyEnvOverrides(cfg, env: [:])
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
    }

    func testLocateUsesExplicitEnvPath() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("myconfig.yaml")
        try sampleYAML.write(to: file, atomically: true, encoding: .utf8)
        defer { try? FileManager.default.removeItem(at: dir) }

        let cfg = ConfigLocator.locate(cwd: dir.path, home: dir.path, env: ["SWITCHBOARD_GO_CONFIG": file.path])
        XCTAssertEqual(cfg?.proxyAPIKey, "test-proxy-key-123")
        XCTAssertEqual(cfg?.configSource, file.path)
    }

    func testLocateFallsBackToCWDConfig() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("config.yaml")
        try sampleYAML.write(to: file, atomically: true, encoding: .utf8)
        defer { try? FileManager.default.removeItem(at: dir) }

        let cfg = ConfigLocator.locate(cwd: dir.path, home: dir.path, env: [:])
        XCTAssertEqual(cfg?.configSource, file.path)
    }

    func testLocateReturnsNilWhenNothingExists() {
        let empty = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true).path
        let cfg = ConfigLocator.locate(cwd: empty, home: empty, env: [:])
        XCTAssertNil(cfg)
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd menu-bar && swift test --filter ConfigLocatorTests`
Expected: compile error — `ProxyConfig` / `ConfigLocator` undefined.

- [ ] **Step 4: Write ConfigLocator.swift**

```swift
import Foundation
import Yams

public struct ProxyConfig: Equatable, Sendable {
    public var baseURL: URL
    public var proxyAPIKey: String
    public var configSource: String
}

public enum ConfigLocator {
    /// Mirrors the Go binary's discovery order (main.go resolveConfigPath):
    /// SWITCHBOARD_GO_CONFIG, then <cwd>/config.yaml|yml, then
    /// ~/.config/switchboard-go/config.yaml, then /etc/switchboard-go/config.yaml.
    public static func locate(
        cwd: String = FileManager.default.currentDirectoryPath,
        home: String = NSHomeDirectory(),
        env: [String: String] = ProcessInfo.processInfo.environment
    ) -> ProxyConfig? {
        var paths: [String] = []
        if let explicit = env["SWITCHBOARD_GO_CONFIG"], !explicit.isEmpty {
            paths.append(explicit)
        }
        paths.append(cwd + "/config.yaml")
        paths.append(cwd + "/config.yml")
        paths.append(home + "/.config/switchboard-go/config.yaml")
        paths.append("/etc/switchboard-go/config.yaml")

        for path in paths where FileManager.default.fileExists(atPath: path) {
            guard let yaml = try? String(contentsOfFile: path, encoding: .utf8),
                  var cfg = parse(yaml: yaml) else { continue }
            cfg = applyEnvOverrides(cfg, env: env)
            cfg.configSource = path
            return cfg
        }
        return nil
    }

    /// Extracts server.listen_addr and server.proxy_api_key. Returns nil when
    /// the YAML is unparseable or no proxy key is configured.
    public static func parse(yaml: String) -> ProxyConfig? {
        guard let doc = try? Yams.load(yaml: yaml) as? [String: Any] else { return nil }
        let server = doc["server"] as? [String: Any] ?? [:]
        let listen = server["listen_addr"] as? String ?? ":8495"
        guard let key = server["proxy_api_key"] as? String, !key.isEmpty else { return nil }
        return ProxyConfig(baseURL: normalize(listen), proxyAPIKey: key, configSource: "")
    }

    /// ":8495" and "0.0.0.0:8495" are reachable on loopback — pin to 127.0.0.1.
    public static func normalize(_ listenAddr: String) -> URL {
        var addr = listenAddr.trimmingCharacters(in: .whitespaces)
        if addr.hasPrefix(":") {
            addr = "127.0.0.1" + addr
        } else if addr.hasPrefix("0.0.0.0:") {
            addr = addr.replacingOccurrences(of: "0.0.0.0:", with: "127.0.0.1:")
        }
        return URL(string: "http://" + addr)!
    }

    /// Matches the Go binary's env precedence (main.go applyEnvOverrides).
    public static func applyEnvOverrides(_ cfg: ProxyConfig?, env: [String: String]) -> ProxyConfig? {
        guard var cfg else { return nil }
        if let key = env["PROXY_API_KEY"], !key.isEmpty {
            cfg.proxyAPIKey = key
        }
        if let listen = env["LISTEN_ADDR"], !listen.isEmpty {
            cfg.baseURL = normalize(listen)
        }
        return cfg
    }
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd menu-bar && swift test --filter ConfigLocatorTests`
Expected: all 9 tests PASS. (Yams resolves on first build; needs network once.)

- [ ] **Step 6: Commit**

```bash
git add menu-bar/Package.swift menu-bar/Package.resolved menu-bar/Sources/SwitchboardMenu/ConfigLocator.swift menu-bar/Tests/SwitchboardMenuTests/ConfigLocatorTests.swift
git commit -s -m "feat(menubar): config discovery mirroring Go precedence with Yams parsing"
```

---

### Task 4: Keychain storage for the proxy key

**Files:**
- Create: `menu-bar/Sources/SwitchboardMenu/KeychainStore.swift`
- Create: `menu-bar/Tests/SwitchboardMenuTests/KeychainStoreTests.swift`

**Interfaces:**
- Produces: `enum KeychainStore` with `static func save(_ key: String)`, `static func load() -> String?`, `static func delete()`. Service: `"switchboard-go-menu-bar"`, account: `"proxy-api-key"`. Task 7 (app init) and Task 8 (Settings) consume this.

- [ ] **Step 1: Write the failing round-trip test**

```swift
import XCTest
@testable import SwitchboardMenu

final class KeychainStoreTests: XCTestCase {
    func testSaveLoadDeleteRoundTrip() {
        KeychainStore.delete()
        XCTAssertNil(KeychainStore.load())

        KeychainStore.save("sk-test-key-abc")
        XCTAssertEqual(KeychainStore.load(), "sk-test-key-abc")

        KeychainStore.save("sk-test-key-updated")
        XCTAssertEqual(KeychainStore.load(), "sk-test-key-updated")

        KeychainStore.delete()
        XCTAssertNil(KeychainStore.load())
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd menu-bar && swift test --filter KeychainStoreTests`
Expected: compile error — `KeychainStore` undefined.

- [ ] **Step 3: Write KeychainStore.swift**

```swift
import Foundation
import Security

/// Stores the manually-entered proxy key. The auto-discovered key from
/// config.yaml is NOT persisted here — it is re-read each launch so config
/// changes are picked up (Keychain wins over config when both exist).
public enum KeychainStore {
    private static let service = "switchboard-go-menu-bar"
    private static let account = "proxy-api-key"

    private static var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
        ]
    }

    public static func save(_ key: String) {
        let data = Data(key.utf8)
        let status = SecItemCopyMatching(baseQuery as CFDictionary, nil)
        if status == errSecSuccess {
            SecItemUpdate(baseQuery as CFDictionary,
                          [kSecValueData as String: data] as CFDictionary)
        } else {
            var add = baseQuery
            add[kSecValueData as String] = data
            SecItemAdd(add as CFDictionary, nil)
        }
    }

    public static func load() -> String? {
        var item: CFTypeRef?
        var query = baseQuery
        query[kSecReturnData as String] = true
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data,
              let value = String(data: data, encoding: .utf8), !value.isEmpty else {
            return nil
        }
        return value
    }

    public static func delete() {
        SecItemDelete(baseQuery as CFDictionary)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd menu-bar && swift test --filter KeychainStoreTests`
Expected: PASS. (SPM test runner is ad-hoc signed on Apple Silicon; the data-protection keychain accepts generic passwords from it without prompting.)

- [ ] **Step 5: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu/KeychainStore.swift menu-bar/Tests/SwitchboardMenuTests/KeychainStoreTests.swift
git commit -s -m "feat(menubar): Keychain storage for manually-entered proxy key"
```

---

### Task 5: APIClient with auth, actions, and 401 typing

**Files:**
- Create: `menu-bar/Sources/SwitchboardMenu/APIClient.swift`
- Create: `menu-bar/Tests/SwitchboardMenuTests/APIClientTests.swift`

**Interfaces:**
- Consumes: `AggregatedUsage`, `StatusResponse`, `MetricsSnapshot` (Task 2).
- Produces:
  - `protocol APIServing`: `func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage`, `func fetchStatus() async throws -> StatusResponse`, `func fetchMetrics() async throws -> MetricsSnapshot`, `func validateKeys() async throws`, `func resetKey(index: Int) async throws`, `func resetAllKeys() async throws`, `func reloadConfig() async throws`
  - `enum APIError: LocalizedError { case unauthorized, http(Int), transport(Error) }`
  - `final class APIClient: APIServing { var baseURL: URL; var apiKey: String; init(baseURL: URL, apiKey: String, session: URLSession = ...) }` — mutable so Settings can update the connection live.
- StatusModel (Task 6) depends on `APIServing` + `APIError`, not the concrete class.

- [ ] **Step 1: Write the failing tests (URLProtocol stub)**

```swift
import XCTest
@testable import SwitchboardMenu

final class MockURLProtocol: URLProtocol {
    nonisolated(unsafe) static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?
    /// URLProtocol does not expose request.httpBody — it arrives as a stream.
    /// startLoading() drains it here so tests can assert on POST bodies.
    nonisolated(unsafe) static var lastBody: Data?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    private static func drainBody(_ request: URLRequest) -> Data? {
        if let data = request.httpBody { return data }
        guard let stream = request.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        let bufferSize = 4096
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
        defer { buffer.deallocate() }
        while stream.hasBytesAvailable {
            let read = stream.read(buffer, maxLength: bufferSize)
            if read <= 0 { break }
            data.append(buffer, count: read)
        }
        return data
    }

    override func startLoading() {
        Self.lastBody = Self.drainBody(request)
        guard let handler = MockURLProtocol.requestHandler else {
            client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

final class APIClientTests: XCTestCase {
    private func makeClient() -> APIClient {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [MockURLProtocol.self]
        return APIClient(baseURL: URL(string: "http://127.0.0.1:8495")!,
                         apiKey: "test-proxy-key",
                         session: URLSession(configuration: config))
    }

    func testFetchUsageAttachesBearerHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/usage")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-proxy-key")
            let body = Data("""
            {"rolling":{"status":"ok","percent":10},"weekly":{"status":"ok","percent":10},
             "monthly":{"status":"ok","percent":10},
             "summary":{"total_keys":1,"available_keys":1,"exhausted_keys":0,
                        "active_sessions":0,"routing_strategy":"session_sticky",
                        "proactive_threshold_percent":95.0},
             "keys":[]}
            """.utf8)
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, body)
        }
        let usage = try await makeClient().fetchUsage(forceRefresh: false)
        XCTAssertEqual(usage.summary.totalKeys, 1)
    }

    func testForceRefreshAppendsQuery() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.query, "refresh=true")
            let body = Data("""
            {"rolling":{"status":"ok","percent":0},"weekly":{"status":"ok","percent":0},
             "monthly":{"status":"ok","percent":0},
             "summary":{"total_keys":0,"available_keys":0,"exhausted_keys":0,
                        "active_sessions":0,"routing_strategy":"s","proactive_threshold_percent":95.0},
             "keys":[]}
            """.utf8)
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, body)
        }
        _ = try await makeClient().fetchUsage(forceRefresh: true)
    }

    func testUnauthorizedThrowsTypedError() async {
        MockURLProtocol.requestHandler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!, Data())
        }
        do {
            _ = try await makeClient().fetchUsage(forceRefresh: false)
            XCTFail("expected unauthorized")
        } catch {
            XCTAssertEqual(error as? APIError, .unauthorized)
        }
    }

    func testHTTPErrorIncludesStatus() async {
        MockURLProtocol.requestHandler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 503, httpVersion: nil, headerFields: nil)!, Data())
        }
        do {
            _ = try await makeClient().fetchUsage(forceRefresh: false)
            XCTFail("expected http error")
        } catch {
            XCTAssertEqual(error as? APIError, .http(503))
        }
    }

    func testResetKeyPostsJSONBody() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/admin/reset-key")
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data())
        }
        try await makeClient().resetKey(index: 0)
        let body = try JSONSerialization.jsonObject(with: MockURLProtocol.lastBody ?? Data()) as? [String: Int]
        XCTAssertEqual(body?["index"], 0)
    }

    func testReloadConfigPosts() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/admin/reload")
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data())
        }
        try await makeClient().reloadConfig()
    }

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        MockURLProtocol.lastBody = nil
    }
}
```

Note: `APIError` must be `Equatable` for the assertions above.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd menu-bar && swift test --filter APIClientTests`
Expected: compile error — `APIClient` / `APIError` undefined.

- [ ] **Step 3: Write APIClient.swift**

```swift
import Foundation

public enum APIError: LocalizedError, Equatable {
    case unauthorized
    case http(Int)
    case transport(String)

    public var errorDescription: String? {
        switch self {
        case .unauthorized:
            return "Invalid proxy API key (401)"
        case .http(let status):
            return "Proxy returned HTTP \(status)"
        case .transport(let message):
            return "Cannot reach proxy: \(message)"
        }
    }
}

public protocol APIServing {
    func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage
    func fetchStatus() async throws -> StatusResponse
    func fetchMetrics() async throws -> MetricsSnapshot
    func validateKeys() async throws
    func resetKey(index: Int) async throws
    func resetAllKeys() async throws
    func reloadConfig() async throws
}

public final class APIClient: APIServing {
    public var baseURL: URL
    public var apiKey: String
    private let session: URLSession

    public init(baseURL: URL, apiKey: String,
                session: URLSession = URLSession(configuration: .ephemeral)) {
        self.baseURL = baseURL
        self.apiKey = apiKey
        self.session = session
    }

    public func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage {
        try await get(path: forceRefresh ? "/usage?refresh=true" : "/usage")
    }

    public func fetchStatus() async throws -> StatusResponse {
        try await get(path: "/admin/status")
    }

    public func fetchMetrics() async throws -> MetricsSnapshot {
        try await get(path: "/dashboard/api/metrics.json")
    }

    public func validateKeys() async throws {
        try await post(path: "/admin/validate-keys")
    }

    public func resetKey(index: Int) async throws {
        try await post(path: "/admin/reset-key", body: ["index": index])
    }

    public func resetAllKeys() async throws {
        try await post(path: "/admin/reset-all-keys")
    }

    public func reloadConfig() async throws {
        try await post(path: "/admin/reload")
    }

    private func get<T: Decodable>(path: String) async throws -> T {
        let (data, response) = try await run(request(path: path))
        return try decode(data, response)
    }

    private func post(path: String, body: [String: Any]? = nil) async throws {
        var request = request(path: path)
        request.httpMethod = "POST"
        if let body {
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        }
        _ = try await run(request)
    }

    private func request(path: String) -> URLRequest {
        var request = URLRequest(url: URL(string: baseURL.absoluteString + path)!)
        request.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        request.timeoutInterval = 10
        return request
    }

    private func run(_ request: URLRequest) async throws -> (Data, URLResponse) {
        do {
            return try await session.data(for: request)
        } catch {
            throw APIError.transport(error.localizedDescription)
        }
    }

    private func decode<T: Decodable>(_ data: Data, _ response: URLResponse) throws -> T {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport("non-HTTP response")
        }
        switch http.statusCode {
        case 401:
            throw APIError.unauthorized
        case 200..<300:
            return try JSONDecoder().decode(T.self, from: data)
        default:
            throw APIError.http(http.statusCode)
        }
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd menu-bar && swift test --filter APIClientTests`
Expected: all 6 tests PASS.

- [ ] **Step 5: Run the whole suite**

Run: `cd menu-bar && swift test`
Expected: all suites green (Models, ConfigLocator, Keychain, APIClient).

- [ ] **Step 6: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu/APIClient.swift menu-bar/Tests/SwitchboardMenuTests/APIClientTests.swift
git commit -s -m "feat(menubar): APIClient with Bearer auth, admin actions, typed errors"
```

---

### Task 6: StatusModel — polling, state transitions, notifications

**Files:**
- Modify: `menu-bar/Sources/SwitchboardMenu/StatusModel.swift` (replace Task 1 stub)
- Create: `menu-bar/Tests/SwitchboardMenuTests/StatusModelTests.swift`

**Interfaces:**
- Consumes: `APIServing`, `APIError`, `AggregatedUsage` (Tasks 2, 5).
- Produces:
  - `enum Transition: Equatable { case exhausted(index: Int), recovered(index: Int), allExhausted }`
  - `protocol Notifying { func post(title: String, body: String) }`
  - `enum MenuAction: String { case validateKeys, resetAllKeys, reloadConfig }`
  - `@MainActor final class StatusModel: ObservableObject`:
    - `@Published public private(set) var usage: AggregatedUsage?`, `@Published public private(set) var snapshot: MetricsSnapshot?`, `@Published public private(set) var lastError: String?`, `@Published public private(set) var needsAttention: Bool` (true on error or any exhausted key — drives the menu bar icon)
    - `let api: APIServing` (class reference, `var`-mutable through `updateConnection`)
    - `init(api: APIServing, notifier: Notifying, interval: TimeInterval = 30)`
    - `func refresh(force: Bool = false) async`, `func startPolling()`, `func stopPolling()`
    - `func perform(_ action: MenuAction) async`
    - `func updateConnection(baseURL: URL, apiKey: String)`
    - `static func diffTransitions(old: AggregatedUsage?, new: AggregatedUsage) -> [Transition]`

- [ ] **Step 1: Write the failing tests**

```swift
import XCTest
@testable import SwitchboardMenu

final class MockNotifier: Notifying {
    var posts: [(title: String, body: String)] = []
    func post(title: String, body: String) {
        posts.append((title, body))
    }
}

final class MockAPI: APIServing {
    var usage: AggregatedUsage?
    var error: Error?
    private(set) var validateCalls = 0
    private(set) var resetAllCalls = 0
    private(set) var reloadCalls = 0

    func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage {
        if let error { throw error }
        return usage!
    }
    func fetchStatus() async throws -> StatusResponse {
        StatusResponse(currentKeyIndex: 0, keys: [], retryExhaustedAfterSeconds: 300, note: "")
    }
    func fetchMetrics() async throws -> MetricsSnapshot {
        MetricsSnapshot(generatedAt: "", httpRequests: [], httpDurations: [],
                        upstreamRequests: [], keyExhaustions: [], keySwitches: [],
                        activeSessions: 0, modelAliases: nil)
    }
    func validateKeys() async throws { validateCalls += 1 }
    func resetKey(index: Int) async throws {}
    func resetAllKeys() async throws { resetAllCalls += 1 }
    func reloadConfig() async throws { reloadCalls += 1 }
}

@MainActor
final class StatusModelTests: XCTestCase {
    private func usage(states: [String]) -> AggregatedUsage {
        let keys = states.enumerated().map { i, state in
            PerKeyUsage(index: i, keyHint: "hint-\(i)", state: state, priority: 1, weight: 1,
                        current: i == 0, eligible: state != "exhausted",
                        retryAfterSeconds: nil,
                        rolling: UsageWindow(status: state, percent: state == "exhausted" ? 99 : 10),
                        weekly: UsageWindow(status: "ok", percent: 10),
                        monthly: UsageWindow(status: "ok", percent: 10))
        }
        return AggregatedUsage(
            rolling: UsageWindow(status: "ok", percent: 10),
            weekly: UsageWindow(status: "ok", percent: 10),
            monthly: UsageWindow(status: "ok", percent: 10),
            summary: UsageSummary(totalKeys: states.count, availableKeys: states.filter { $0 == "available" }.count,
                                  exhaustedKeys: states.filter { $0 == "exhausted" }.count,
                                  activeSessions: 0, routingStrategy: "session_sticky",
                                  proactiveThresholdPercent: 95, poolUsage: nil),
            keys: keys)
    }

    func testDiffExhaustionAndRecovery() {
        let old = usage(states: ["available", "available"])
        let new = usage(states: ["exhausted", "available"])
        let transitions = StatusModel.diffTransitions(old: old, new: new)
        XCTAssertEqual(transitions, [.exhausted(index: 0)])
    }

    func testDiffRecovery() {
        let old = usage(states: ["exhausted", "available"])
        let new = usage(states: ["available", "available"])
        XCTAssertEqual(StatusModel.diffTransitions(old: old, new: new), [.recovered(index: 0)])
    }

    func testDiffNoTransitionsWhenUnchanged() {
        let old = usage(states: ["exhausted", "exhausted"])
        let new = usage(states: ["exhausted", "exhausted"])
        XCTAssertTrue(StatusModel.diffTransitions(old: old, new: new).isEmpty)
    }

    func testDiffAllExhaustedFiresOnce() {
        let old = usage(states: ["available", "available"])
        let new = usage(states: ["exhausted", "exhausted"])
        let transitions = StatusModel.diffTransitions(old: old, new: new)
        XCTAssertEqual(transitions, [.exhausted(index: 0), .exhausted(index: 1), .allExhausted])
    }

    func testFirstPollReportsPreExistingExhaustion() {
        let new = usage(states: ["exhausted"])
        let transitions = StatusModel.diffTransitions(old: nil, new: new)
        XCTAssertEqual(transitions, [.exhausted(index: 0), .allExhausted])
    }

    func testRefreshPopulatesUsageAndNotifies() async {
        let api = MockAPI()
        api.usage = usage(states: ["available"])
        let notifier = MockNotifier()
        let model = StatusModel(api: api, notifier: notifier, interval: 999)
        await model.refresh()
        XCTAssertNotNil(model.usage)
        XCTAssertNotNil(model.snapshot)
        XCTAssertNil(model.lastError)
        XCTAssertFalse(model.needsAttention)
        XCTAssertTrue(notifier.posts.isEmpty)
    }

    func testRefreshNotifiesOnExhaustion() async {
        let api = MockAPI()
        api.usage = usage(states: ["available"])
        let notifier = MockNotifier()
        let model = StatusModel(api: api, notifier: notifier, interval: 999)
        await model.refresh()

        api.usage = usage(states: ["exhausted"])
        await model.refresh()
        XCTAssertEqual(notifier.posts.count, 2) // exhausted + allExhausted
        XCTAssertTrue(model.needsAttention)
    }

    func testRefreshUnauthorizedMarksNotConfigured() async {
        let api = MockAPI()
        api.error = APIError.unauthorized
        let model = StatusModel(api: api, notifier: MockNotifier(), interval: 999)
        await model.refresh()
        XCTAssertNotNil(model.lastError)
        XCTAssertFalse(model.isConfigured)
        XCTAssertTrue(model.needsAttention)
    }

    func testPerformResetAllThenRefreshes() async {
        let api = MockAPI()
        api.usage = usage(states: ["available"])
        let model = StatusModel(api: api, notifier: MockNotifier(), interval: 999)
        await model.perform(.resetAllKeys)
        XCTAssertEqual(api.resetAllCalls, 1)
    }

    func testUpdateConnectionMutatesClient() {
        let client = APIClient(baseURL: URL(string: "http://127.0.0.1:8495")!, apiKey: "a")
        let model = StatusModel(api: client, notifier: MockNotifier(), interval: 999)
        model.updateConnection(baseURL: URL(string: "http://127.0.0.1:9000")!, apiKey: "b")
        XCTAssertEqual(client.baseURL.absoluteString, "http://127.0.0.1:9000")
        XCTAssertEqual(client.apiKey, "b")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd menu-bar && swift test --filter StatusModelTests`
Expected: compile errors — `Transition`, `Notifying`, `MenuAction`, members undefined (stub is empty).

- [ ] **Step 3: Implement StatusModel.swift (replacing the stub)**

```swift
import Foundation
import Combine

public enum Transition: Equatable {
    case exhausted(index: Int)
    case recovered(index: Int)
    case allExhausted
}

public protocol Notifying {
    func post(title: String, body: String)
}

public enum MenuAction: String {
    case validateKeys
    case resetAllKeys
    case reloadConfig
}

@MainActor
public final class StatusModel: ObservableObject {
    @Published public private(set) var usage: AggregatedUsage?
    @Published public private(set) var snapshot: MetricsSnapshot?
    @Published public private(set) var lastError: String?
    @Published public private(set) var needsAttention = false
    @Published public var isConfigured = true

    public let api: APIServing
    private let notifier: Notifying
    public var interval: TimeInterval
    private var timer: Timer?

    public init(api: APIServing, notifier: Notifying, interval: TimeInterval = 30) {
        self.api = api
        self.notifier = notifier
        self.interval = interval
    }

    public func refresh(force: Bool = false) async {
        do {
            let fresh = try await api.fetchUsage(forceRefresh: force)
            let transitions = Self.diffTransitions(old: usage, new: fresh)
            usage = fresh
            snapshot = try? await api.fetchMetrics()
            lastError = nil
            isConfigured = true
            needsAttention = fresh.keys.contains { $0.state == "exhausted" }
            for transition in transitions {
                notify(transition)
            }
        } catch {
            lastError = error.localizedDescription
            isConfigured = !(error is APIError && (error as? APIError) == .unauthorized)
            needsAttention = true
        }
    }

    public func startPolling() {
        guard timer == nil else { return }
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
            Task { @MainActor [weak self] in
                await self?.refresh()
            }
        }
        Task { await refresh() }
    }

    public func stopPolling() {
        timer?.invalidate()
        timer = nil
    }

    public func perform(_ action: MenuAction) async {
        do {
            switch action {
            case .validateKeys:
                try await api.validateKeys()
            case .resetAllKeys:
                try await api.resetAllKeys()
            case .reloadConfig:
                try await api.reloadConfig()
            }
            await refresh(force: true)
        } catch {
            lastError = error.localizedDescription
        }
    }

    public func updateConnection(baseURL: URL, apiKey: String) {
        guard let client = api as? APIClient else { return }
        client.baseURL = baseURL
        client.apiKey = apiKey
    }

    /// Pure diff of per-key states between the previous and new usage payload.
    static func diffTransitions(old: AggregatedUsage?, new: AggregatedUsage) -> [Transition] {
        var transitions: [Transition] = []
        let oldByIndex = Dictionary(old?.keys.map { ($0.index, $0) } ?? [], uniquingKeysWith: { first, _ in first })
        for key in new.keys {
            let previous = oldByIndex[key.index]?.state
            if key.state == "exhausted" && previous != "exhausted" {
                transitions.append(.exhausted(index: key.index))
            }
            if key.state == "available" && previous == "exhausted" {
                transitions.append(.recovered(index: key.index))
            }
        }
        let allExhaustedNow = !new.keys.isEmpty && new.keys.allSatisfy { $0.state == "exhausted" }
        let allExhaustedBefore = !(old?.keys.isEmpty ?? true) && (old?.keys.allSatisfy { $0.state == "exhausted" } ?? false)
        if allExhaustedNow && !allExhaustedBefore {
            transitions.append(.allExhausted)
        }
        return transitions
    }

    private func notify(_ transition: Transition) {
        switch transition {
        case .exhausted(let index):
            notifier.post(title: "Key \(index) exhausted",
                          body: "Switchboard Go rotated traffic away from key \(index).")
        case .recovered(let index):
            notifier.post(title: "Key \(index) recovered",
                          body: "Key \(index) is available again.")
        case .allExhausted:
            notifier.post(title: "All keys exhausted",
                          body: "Every upstream key is out of quota. Requests will 429 until a key resets.")
        }
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd menu-bar && swift test --filter StatusModelTests`
Expected: all 10 tests PASS.

- [ ] **Step 5: Full suite**

Run: `cd menu-bar && swift test`
Expected: everything green.

- [ ] **Step 6: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu/StatusModel.swift menu-bar/Tests/SwitchboardMenuTests/StatusModelTests.swift
git commit -s -m "feat(menubar): StatusModel with polling, transition diffing, and notifier hooks"
```

---

### Task 7: App shell, menu bar label, and popover UI

**Files:**
- Modify: `menu-bar/Sources/SwitchboardMenu/SwitchboardMenuApp.swift` (replace placeholder)
- Create: `menu-bar/Sources/SwitchboardMenu/Formatters.swift`
- Create: `menu-bar/Sources/SwitchboardMenu/PopoverContentView.swift` (contains `PopoverContentView` + `StatusLabelView` + `KeyRowView` + `StatsSection`)

**Interfaces:**
- Consumes: `StatusModel` (Task 6), `ConfigLocator`/`KeychainStore` (Tasks 3–4), models (Task 2).
- Produces: the running app. `SwitchboardMenuApp.init` builds the `APIClient` as: base URL = `ConfigLocator.locate()` (env-overridden) else `http://127.0.0.1:8495`; key = `KeychainStore.load()` else discovered `proxyAPIKey` else `""`. `AppDelegate.applicationDidFinishLaunching` calls `SystemNotifier.shared.requestAuthorization()` (Task 9 provides `SystemNotifier`; define a temporary `DebugNotifier` conforming to `Notifying` in this task if Task 9 has not run, then swap in Task 9) and `model.startPolling()`.
- No unit tests (SwiftUI views); verified by build + launch smoke test.

- [ ] **Step 1: Write Formatters.swift**

```swift
import Foundation

enum Formatters {
    static func percent(_ value: Double) -> String {
        String(format: "%.0f%%", value.rounded())
    }

    /// Go zero-time ("0001-01-01T00:00:00Z") means "no reset known".
    static func isMeaningfulReset(_ iso: String?) -> Bool {
        guard let iso, !iso.isEmpty else { return false }
        return !iso.hasPrefix("0001-01-01")
    }

    static func countdown(from iso: String?, now: Date = Date()) -> String? {
        guard isMeaningfulReset(iso), let target = ISO8601DateFormatter.parse(iso!) else {
            return nil
        }
        let diff = target.timeIntervalSince(now)
        if diff <= 0 { return "reset due" }
        let hours = Int(diff) / 3600
        let minutes = (Int(diff) % 3600) / 60
        if hours > 0 { return "in \(hours)h \(minutes)m" }
        return "in \(minutes)m"
    }

    static func timeAgo(_ iso: String) -> String {
        guard let date = ISO8601DateFormatter.parse(iso) else { return "" }
        let diff = Date().timeIntervalSince(date)
        if diff < 60 { return "just now" }
        let minutes = Int(diff) / 60
        if minutes < 60 { return "\(minutes)m ago" }
        return "\(minutes / 60)h ago"
    }

    static func stateColor(_ state: String) -> String {
        switch state {
        case "available": return "green"
        case "exhausted": return "red"
        default: return "gray"
        }
    }
}

extension ISO8601DateFormatter {
    private static let flexible: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let plain: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    /// Server emits both "…Z" and "…Z" with fractional seconds (Go time.RFC3339 vs RFC3339Nano).
    static func parse(_ iso: String) -> Date? {
        flexible.date(from: iso) ?? plain.date(from: iso)
    }
}
```

- [ ] **Step 2: Write PopoverContentView.swift**

```swift
import SwiftUI

struct StatusLabelView: View {
    @ObservedObject var model: StatusModel

    var body: some View {
        if model.lastError != nil {
            Label("SB?", systemImage: "exclamationmark.triangle.fill")
        } else if let usage = model.usage {
            Label(labelText(usage), systemImage: iconName(usage))
        } else {
            Label("SB", systemImage: "arrow.triangle.swap")
        }
    }

    private func labelText(_ usage: AggregatedUsage) -> String {
        let available = usage.summary.availableKeys
        let total = usage.summary.totalKeys
        let percent = Int(usage.rolling.percent.rounded())
        return "\(percent)% \(available)/\(total)"
    }

    private func iconName(_ usage: AggregatedUsage) -> String {
        if usage.summary.totalKeys > 0 && usage.summary.exhaustedKeys == usage.summary.totalKeys {
            return "exclamationmark.triangle.fill"
        }
        if usage.keys.contains(where: { $0.state == "exhausted" }) {
            return "exclamationmark.triangle"
        }
        return "arrow.triangle.swap"
    }
}

struct KeyRowView: View {
    let key: PerKeyUsage

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(stateColor)
                .frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(key.keyHint ?? "key \(key.index)")
                        .font(.system(.caption, design: .monospaced))
                    if key.current {
                        Image(systemName: "star.fill")
                            .font(.caption2)
                            .foregroundColor(.blue)
                    }
                    Spacer()
                    Text(stateText)
                        .font(.caption2)
                        .foregroundColor(stateColor)
                }
                windowBar("R", key.rolling)
                windowBar("W", key.weekly)
                windowBar("M", key.monthly)
                footerLine
            }
        }
        .padding(.vertical, 2)
    }

    private var stateColor: Color {
        switch Formatters.stateColor(key.state) {
        case "green": return .green
        case "red": return .red
        default: return .gray
        }
    }

    private var stateText: String {
        if key.state == "exhausted", let retry = key.retryAfterSeconds {
            return "exhausted · retry \(retry)s"
        }
        return key.state
    }

    private var footerLine: String {
        var parts: [String] = []
        if let reset = Formatters.countdown(from: key.rolling.resetsAt) {
            parts.append("resets \(reset)")
        }
        if let checked = key.lastCheckedAt {
            parts.append("checked \(Formatters.timeAgo(checked))")
        }
        if let error = key.error {
            parts.append(error)
        }
        return parts.joined(separator: " · ")
    }

    private func windowBar(_ label: String, _ window: UsageWindow) -> some View {
        HStack(spacing: 4) {
            Text(label)
                .font(.caption2)
                .foregroundColor(.secondary)
                .frame(width: 10)
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule().fill(Color.secondary.opacity(0.2))
                    Capsule()
                        .fill(barColor(window.percent))
                        .frame(width: geo.size.width * min(1, max(0, window.percent / 100)))
                }
            }
            .frame(height: 5)
            Text(Formatters.percent(window.percent))
                .font(.caption2)
                .monospacedDigit()
                .frame(width: 34, alignment: .trailing)
        }
    }

    private func barColor(_ percent: Double) -> Color {
        if percent >= 95 { return .red }
        if percent >= 70 { return .orange }
        return .green
    }
}

struct StatsSection: View {
    let snapshot: MetricsSnapshot?

    var body: some View {
        if let snap = snapshot {
            let exhaustions = snap.keyExhaustions.reduce(0) { $0 + $1.count }
            let switches = snap.keySwitches.reduce(0) { $0 + $1.count }
            HStack(spacing: 12) {
                Label("\(snap.activeSessions) sessions", systemImage: "person.2")
                Label("\(exhaustions) exhaustions", systemImage: "bolt.slash")
                Label("\(switches) switches", systemImage: "arrow.left.arrow.right")
            }
            .font(.caption)
            .foregroundColor(.secondary)
        }
    }
}

struct PopoverContentView: View {
    @EnvironmentObject var model: StatusModel
    @State private var showSettings = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            if let error = model.lastError {
                Text(error)
                    .font(.caption)
                    .foregroundColor(.red)
            }
            keysSection
            StatsSection(snapshot: model.snapshot)
            Divider()
            actions
            if showSettings {
                SettingsSection()
            }
        }
        .padding(12)
        .frame(width: 320)
        .task { await model.refresh() }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Switchboard Go")
                .font(.headline)
            if let usage = model.usage {
                Text("\(usage.summary.availableKeys)/\(usage.summary.totalKeys) keys available · rolling \(Formatters.percent(usage.rolling.percent))")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
    }

    @ViewBuilder
    private var keysSection: some View {
        if let usage = model.usage, !usage.keys.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                ForEach(usage.keys) { key in
                    KeyRowView(key: key)
                }
            }
        } else if model.usage != nil {
            Text("No keys configured").font(.caption).foregroundColor(.secondary)
        } else {
            ProgressView().controlSize(.small)
        }
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Button("Refresh") { Task { await model.refresh(force: true) } }
                Button("Validate") { Task { await model.perform(.validateKeys) } }
                Button("Reset all") { Task { await model.perform(.resetAllKeys) } }
                Button("Reload") { Task { await model.perform(.reloadConfig) } }
            }
            .controlSize(.small)

            HStack {
                Button("Open Dashboard") {
                    if let base = (model.api as? APIClient)?.baseURL,
                       let url = URL(string: base.absoluteString + "/dashboard/") {
                        NSWorkspace.shared.open(url)
                    }
                }
                Button("Settings") { showSettings.toggle() }
                Spacer()
                Button("Quit") { NSApplication.shared.terminate(nil) }
            }
            .controlSize(.small)
        }
    }
}
```

- [ ] **Step 3: Write the app shell (SwitchboardMenuApp.swift)**

```swift
import SwiftUI
import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // Task 9 replaces DebugNotifier with SystemNotifier and moves the
        // authorization request here. For now nothing to do.
    }
}

/// Temporary notifier so the app compiles before Task 9 lands.
struct DebugNotifier: Notifying {
    func post(title: String, body: String) {
        NSLog("switchboard-menubar: %@ — %@", title, body)
    }
}

@main
struct SwitchboardMenuApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model: StatusModel

    init() {
        let env = ProcessInfo.processInfo.environment
        let discovered = ConfigLocator.locate(env: env)
        let baseURL = discovered?.baseURL ?? URL(string: "http://127.0.0.1:8495")!
        let key = KeychainStore.load() ?? discovered?.proxyAPIKey ?? ""
        let client = APIClient(baseURL: baseURL, apiKey: key)
        _model = StateObject(wrappedValue: StatusModel(api: client, notifier: DebugNotifier(), interval: 30))
        // Idempotent (guarded); safe to call here — App.init runs on the main actor.
        model.startPolling()
    }

    var body: some Scene {
        MenuBarExtra {
            PopoverContentView()
                .environmentObject(model)
        } label: {
            StatusLabelView(model: model)
        }
        .menuBarExtraStyle(.window)
    }
}
```

(`StatusModel` already exposes `snapshot` — added in Task 6 — which `StatsSection` consumes. The `try?` in `refresh()` means a metrics fetch failure never breaks the usage display.)

- [ ] **Step 4: Build and launch smoke test**

Run:
```bash
cd menu-bar && swift build -c release
.build/release/SwitchboardMenu & sleep 2
pgrep -x SwitchboardMenu && echo "RUNNING"
pkill -x SwitchboardMenu
```
Expected: `RUNNING` printed; a menu bar item labeled like `SB` (or `0% 0/0`) appears; process exits on pkill. (Visual confirmation of data happens in Task 10 against the live proxy.)

- [ ] **Step 5: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu menu-bar/Tests
git commit -s -m "feat(menubar): MenuBarExtra app shell with popover, key rows, and actions"
```

---

### Task 8: Settings section (connection, interval, launch-at-login)

**Files:**
- Create: `menu-bar/Sources/SwitchboardMenu/SettingsSection.swift`
- Modify: `menu-bar/Sources/SwitchboardMenu/StatusModel.swift` (add `@Published public var pollInterval: TimeInterval` with didSet restarting the timer; `updateConnection` already exists)
- Modify: `menu-bar/Sources/SwitchboardMenu/PopoverContentView.swift` (already toggles `showSettings`)

**Interfaces:**
- Consumes: `KeychainStore.save/delete/load`, `StatusModel.updateConnection(baseURL:apiKey:)`, `SMAppService.mainApp` (macOS 13).
- Produces: `struct SettingsSection: View` — `TextField` base URL, `SecureField` proxy key, Save/Clear buttons, poll-interval `Picker` (15s/30s/60s, `@AppStorage("sb_menubar_interval")` default `30`), launch-at-login `Toggle` backed by `SMAppService.mainApp.status` / `register()` / `unregister()`, and a footer showing `model.api as? APIClient`'s base URL + the discovered `configSource` when available.

- [ ] **Step 1: Write SettingsSection.swift**

```swift
import SwiftUI
import ServiceManagement

struct SettingsSection: View {
    @EnvironmentObject var model: StatusModel
    @AppStorage("sb_menubar_interval") private var intervalMinutes = 0.5 // minutes; 0.5 = 30s
    @State private var baseURL: String = ""
    @State private var apiKey: String = ""
    @State private var savedFlash = false
    @State private var loginItemEnabled = SMAppService.mainApp.status == .enabled

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Divider()
            Text("Settings").font(.subheadline).bold()

            LabeledContent("Proxy URL") {
                TextField("http://127.0.0.1:8495", text: $baseURL)
                    .textFieldStyle(.roundedBorder)
            }
            LabeledContent("Proxy key") {
                SecureField("server.proxy_api_key", text: $apiKey)
                    .textFieldStyle(.roundedBorder)
            }

            LabeledContent("Poll interval") {
                Picker("", selection: $intervalMinutes) {
                    Text("15s").tag(0.25)
                    Text("30s").tag(0.5)
                    Text("60s").tag(1.0)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .onChange(of: intervalMinutes) { newValue in
                    model.pollInterval = newValue * 60
                }
            }

            Toggle("Launch at login", isOn: $loginItemEnabled)
                .onChange(of: loginItemEnabled) { enabled in
                do {
                    if enabled {
                        try SMAppService.mainApp.register()
                    } else {
                        try SMAppService.mainApp.unregister()
                    }
                } catch {
                    loginItemEnabled = SMAppService.mainApp.status == .enabled
                }
            }

            HStack {
                Button("Save") {
                    let url = URL(string: baseURL) ?? URL(string: "http://127.0.0.1:8495")!
                    model.updateConnection(baseURL: url, apiKey: apiKey)
                    if !apiKey.isEmpty {
                        KeychainStore.save(apiKey)
                    }
                    savedFlash = true
                    Task {
                        try? await Task.sleep(nanoseconds: 1_500_000_000)
                        savedFlash = false
                        await model.refresh(force: true)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)

                Button("Clear key") {
                    KeychainStore.delete()
                    apiKey = ""
                }
                .controlSize(.small)

                if savedFlash {
                    Text("Saved").font(.caption).foregroundColor(.green)
                }
                Spacer()
            }

            if let client = model.api as? APIClient {
                Text("Connected to \(client.baseURL.absoluteString) · key source: \(KeychainStore.load() != nil ? "Keychain" : "config.yaml")")
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }
        }
        .onAppear {
            if let client = model.api as? APIClient {
                baseURL = client.baseURL.absoluteString
            }
            apiKey = KeychainStore.load() ?? ""
        }
    }
}
```

- [ ] **Step 2: Make poll interval live-tunable in StatusModel**

Add to `StatusModel` (replacing the plain `public var interval`):

```swift
    @Published public var pollInterval: TimeInterval {
        didSet {
            guard pollInterval != oldValue else { return }
            restartTimer()
        }
    }
```

Constructor sets `self.pollInterval = interval` (drop the stored `interval`). Add:

```swift
    private func restartTimer() {
        stopPolling()
        timer = Timer.scheduledTimer(withTimeInterval: pollInterval, repeats: true) { [weak self] _ in
            Task { @MainActor [weak self] in
                await self?.refresh()
            }
        }
    }
```

`startPolling()` becomes:

```swift
    public func startPolling() {
        guard timer == nil else { return }
        restartTimer()
        Task { await refresh() }
    }
```

(Existing StatusModelTests compile unchanged — they never touch `interval` after init.)

- [ ] **Step 3: Build + full test suite**

Run: `cd menu-bar && swift build && swift test`
Expected: build OK, all suites green.

- [ ] **Step 4: Launch smoke test**

Same commands as Task 7 Step 4. Expected: popover → Settings reveals the form; saving updates the connection (verify visually in Task 10).

- [ ] **Step 5: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu
git commit -s -m "feat(menubar): settings section with keychain save, interval, and login item"
```

---

### Task 9: System notifications on transitions

**Files:**
- Create: `menu-bar/Sources/SwitchboardMenu/Notifications.swift`
- Modify: `menu-bar/Sources/SwitchboardMenu/SwitchboardMenuApp.swift` (swap `DebugNotifier` → `SystemNotifier`; request authorization in `applicationDidFinishLaunching`)

**Interfaces:**
- Consumes: `Notifying` protocol (Task 6).
- Produces: `final class SystemNotifier: Notifying { static let shared = SystemNotifier(); func requestAuthorization() }` implementing `post(title:body:)` via `UNUserNotificationCenter`.

- [ ] **Step 1: Write Notifications.swift**

```swift
import Foundation
import UserNotifications

final class SystemNotifier: Notifying {
    static let shared = SystemNotifier()
    private let center = UNUserNotificationCenter.current()

    func requestAuthorization() {
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }

    func post(title: String, body: String) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        center.add(request)
    }
}
```

- [ ] **Step 2: Wire into the app shell**

In `SwitchboardMenuApp.swift`: delete `DebugNotifier`; in `init()` pass `notifier: SystemNotifier.shared`; extend `AppDelegate`:

```swift
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        SystemNotifier.shared.requestAuthorization()
    }
}
```

- [ ] **Step 3: Build + suite + smoke**

Run: `cd menu-bar && swift build && swift test` then launch smoke as in Task 7.
Expected: green suite; app launches. macOS may show a one-time notifications permission prompt on first transition notification.

- [ ] **Step 4: Commit**

```bash
git add menu-bar/Sources/SwitchboardMenu
git commit -s -m "feat(menubar): system notifications on key exhaustion, recovery, and all-exhausted"
```

---

### Task 10: End-to-end verification + docs

**Files:**
- Create: `docs/menubar.md`
- Modify: `README.md` (feature bullet + short section)

**Interfaces:**
- Consumes: everything above; the live proxy on `127.0.0.1:8495` built from repo main.

- [ ] **Step 1: Full clean verification**

```bash
cd menu-bar
swift build -c release
swift test
```
Expected: build OK; every test green.

- [ ] **Step 2: Live end-to-end run**

```bash
# ensure the rebuilt proxy is running (see README dashboard section):
.build/release/SwitchboardMenu & sleep 3
pgrep -x SwitchboardMenu
```

Manual checklist (user-visible behaviors — walk through with the human):
1. Menu bar shows e.g. `7% 2/2` with the swap icon; popover lists both keys with masked hints (`sk-5R6d…z28B`), state dots, R/W/M bars matching the web dashboard numbers.
2. Stats row shows session/exhaustion/switch counters consistent with `curl -s http://127.0.0.1:8495/dashboard/api/metrics.json`.
3. `Refresh`, `Validate`, `Reset all`, `Reload` buttons act on the proxy (verify via dashboard/`curl /admin/status`).
4. "Open Dashboard" opens `http://127.0.0.1:8495/dashboard/` in the default browser.
5. Settings → wrong key + Save → popover shows "Invalid proxy API key (401)"; correct key restored → data returns.
6. Exhaust a key (or `curl -X POST .../admin/reset-key` cycles) → macOS notification fires "Key N exhausted"; recover → "Key N recovered".
7. Kill the proxy → popover shows transport error banner; restart proxy → auto-recovers within one poll interval.
8. Quit works; relaunch works.

- [ ] **Step 3: Write docs/menubar.md**

```markdown
# macOS Menu Bar App

Native companion to the Switchboard Go proxy: live key/quota state in the
menu bar, notifications on exhaustion/recovery, and admin actions without
opening a browser.

## Build

    cd menu-bar
    swift build -c release

Binary: `menu-bar/.build/release/SwitchboardMenu`. Requires macOS 13+.

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
```

- [ ] **Step 4: README update**

Add to the README feature list, after the dashboard bullet:

```markdown
- **macOS Menu Bar App (`menu-bar/`)**: Native live status, notifications on key exhaustion, and admin actions; build with `swift build -c release` (see [docs/menubar.md](docs/menubar.md))
```

And after the "Web dashboard" section:

```markdown
## macOS menu bar app

A native menu bar companion lives in `menu-bar/` (SwiftPM, macOS 13+):

    cd menu-bar && swift build -c release
    .build/release/SwitchboardMenu &

It auto-discovers the proxy via the same config precedence as the server and
shows live quota state, notifications, and admin actions. See
[docs/menubar.md](docs/menubar.md).
```

- [ ] **Step 5: Commit**

```bash
git add README.md docs/menubar.md
git commit -s -m "docs(menubar): document macOS menu bar app build and usage"
```

---

## Risks / Notes

- **Yams needs network** on first `swift build` (SPM resolve). If offline, Task 3 blocks — everything else is standard-library only.
- **Keychain in tests**: SPM's ad-hoc-signed test runner generally writes data-protection keychain items without prompts on Apple Silicon. If a prompt or `errSecMissingEntitlement` appears, run `swift test` once from Xcode-less CLI with `-Xswiftc` no changes needed — worst case, guard the Keychain test with `XCTSkipUnless(KeychainStore.isAvailable())` helper that probes with a round-trip.
- **MenuBarExtra label refresh**: SwiftUI re-renders the label via `@ObservedObject`; if the label does not live-update on some macOS versions, fall back to a tiny `NSStatusItem` wrapper — same `StatusModel`, different host view. Decide only if observed broken in Task 10's checklist.
- **Swift 6 concurrency**: package pins `swift-tools-version:5.9` (Swift 5 mode) deliberately. Do not bump to 6 without addressing `@MainActor` in `App.init` and `nonisolated(unsafe)` in `MockURLProtocol` (already annotated for forward-compat).
- The app polls `/usage` every 30s — same cadence as the server's own upstream poller; it does not use `?refresh=true` on the automatic path (only manual Refresh), so it never triggers upstream load beyond one extra local call.
