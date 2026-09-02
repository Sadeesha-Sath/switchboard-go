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
