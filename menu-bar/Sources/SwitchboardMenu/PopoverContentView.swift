import SwiftUI
import AppKit

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
                Text(footerLine)
                    .font(.caption2)
                    .foregroundColor(.secondary)
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
