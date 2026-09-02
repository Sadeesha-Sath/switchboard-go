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
