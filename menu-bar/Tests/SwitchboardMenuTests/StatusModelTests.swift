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
