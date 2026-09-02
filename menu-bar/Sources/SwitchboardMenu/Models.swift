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
    public var priority: Int = 0
    public var weight: Int = 0
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

    public init(
        index: Int,
        keyHint: String? = nil,
        state: String,
        priority: Int = 0,
        weight: Int = 0,
        last429Time: String? = nil,
        current: Bool,
        eligible: Bool,
        retryAfterSeconds: Int? = nil
    ) {
        self.index = index
        self.keyHint = keyHint
        self.state = state
        self.priority = priority
        self.weight = weight
        self.last429Time = last429Time
        self.current = current
        self.eligible = eligible
        self.retryAfterSeconds = retryAfterSeconds
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        index = try c.decode(Int.self, forKey: .index)
        keyHint = try c.decodeIfPresent(String.self, forKey: .keyHint)
        state = try c.decode(String.self, forKey: .state)
        priority = try c.decodeIfPresent(Int.self, forKey: .priority) ?? 0
        weight = try c.decodeIfPresent(Int.self, forKey: .weight) ?? 0
        last429Time = try c.decodeIfPresent(String.self, forKey: .last429Time)
        current = try c.decode(Bool.self, forKey: .current)
        eligible = try c.decode(Bool.self, forKey: .eligible)
        retryAfterSeconds = try c.decodeIfPresent(Int.self, forKey: .retryAfterSeconds)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(index, forKey: .index)
        try c.encodeIfPresent(keyHint, forKey: .keyHint)
        try c.encode(state, forKey: .state)
        try c.encode(priority, forKey: .priority)
        try c.encode(weight, forKey: .weight)
        try c.encodeIfPresent(last429Time, forKey: .last429Time)
        try c.encode(current, forKey: .current)
        try c.encode(eligible, forKey: .eligible)
        try c.encodeIfPresent(retryAfterSeconds, forKey: .retryAfterSeconds)
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
