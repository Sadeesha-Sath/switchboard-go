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
    @Published public var pollInterval: TimeInterval {
        didSet {
            guard pollInterval != oldValue else { return }
            guard timer != nil else { return }
            restartTimer()
        }
    }
    private var timer: Timer?

    public init(api: APIServing, notifier: Notifying, interval: TimeInterval = 30) {
        self.api = api
        self.notifier = notifier
        self.pollInterval = interval
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
            isConfigured = !((error as? APIError) == .unauthorized)
            needsAttention = true
        }
    }

    private func restartTimer() {
        stopPolling()
        timer = Timer.scheduledTimer(withTimeInterval: pollInterval, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.refresh()
            }
        }
    }

    public func startPolling() {
        guard timer == nil else { return }
        restartTimer()
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
