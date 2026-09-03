import SwiftUI
import AppKit

// MARK: - Menu Bar Status Label
struct StatusLabelView: View {
    @ObservedObject var model: StatusModel

    var body: some View {
        HStack(spacing: 5) {
            if model.lastError != nil || model.usage == nil {
                // State 4: Unreachable — dimmed glyph and an em dash
                Image(systemName: "arrow.triangle.swap")
                    .foregroundColor(SBTheme.neutral500)
                Text("—")
                    .font(.system(size: 12))
                    .monospacedDigit()
                    .foregroundColor(SBTheme.neutral500)
            } else if let usage = model.usage {
                let available = usage.summary.availableKeys
                let total = usage.summary.totalKeys
                let percent = Int(usage.rolling.percent.rounded())

                if total > 0 && available == 0 {
                    // State 3: All keys exhausted — filled glyph, label in accent
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundColor(SBTheme.accent300)
                    Text("\(percent)% 0/\(total)")
                        .font(.system(size: 12))
                        .monospacedDigit()
                        .foregroundColor(SBTheme.accent300)
                } else if usage.keys.contains(where: { $0.state == "exhausted" }) {
                    // State 2: One key exhausted — hollow gold warning glyph
                    Image(systemName: "exclamationmark.triangle")
                        .foregroundColor(SBTheme.accent400)
                    Text("\(percent)% \(available)/\(total)")
                        .font(.system(size: 12))
                        .monospacedDigit()
                } else {
                    // State 1: Healthy — rolling % and available/total
                    Image(systemName: "arrow.triangle.swap")
                    Text("\(percent)% \(available)/\(total)")
                        .font(.system(size: 12))
                        .monospacedDigit()
                }
            }
        }
    }
}

// MARK: - Key Row View
struct KeyRowView: View {
    let key: PerKeyUsage

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header Row: Index, Star, Hint, Badge
            HStack(spacing: 8) {
                Text("\(key.index)")
                    .font(.system(size: 11))
                    .monospacedDigit()
                    .foregroundColor(SBTheme.neutral600)

                if key.current {
                    Image(systemName: "star.fill")
                        .font(.system(size: 11))
                        .foregroundColor(SBTheme.accent400)
                }

                Text(key.keyHint ?? "key \(key.index)")
                    .font(.sbHeading(size: 15))
                    .foregroundColor(key.state == "exhausted" ? SBTheme.neutral400 : SBTheme.neutral100)
                    .lineLimit(1)

                Spacer()

                badgeView
            }

            // Usage progress bars
            VStack(spacing: 7) {
                usageBar(label: "5-HOUR", percent: key.rolling.percent, isExhausted: key.state == "exhausted")
                usageBar(label: "WEEKLY", percent: key.weekly.percent, isExhausted: false)
                usageBar(label: "MONTHLY", percent: key.monthly.percent, isExhausted: false)
            }
            .padding(.top, 11)

            // Footer info
            Text(footerText)
                .font(.system(size: 10.5))
                .monospacedDigit()
                .foregroundColor(SBTheme.neutral600)
                .padding(.top, 10)
        }
        .padding(EdgeInsets(top: 11, leading: 12, bottom: 10, trailing: 12))
        .background(Color.clear)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(key.current ? SBTheme.accent600 : SBTheme.borderMuted, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 4))
    }

    @ViewBuilder
    private var badgeView: some View {
        if key.state == "available" {
            Text("AVAILABLE")
                .font(.system(size: 9.5, weight: .regular))
                .tracking(0.9)
                .foregroundColor(SBTheme.accent400)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .overlay(
                    RoundedRectangle(cornerRadius: 3)
                        .stroke(SBTheme.accent400, lineWidth: 1)
                )
        } else if key.state == "exhausted" {
            let retry = key.retryAfterSeconds != nil ? " · RETRY \(key.retryAfterSeconds!)s" : ""
            Text("EXHAUSTED\(retry)")
                .font(.system(size: 9.5, weight: .regular))
                .tracking(0.9)
                .monospacedDigit()
                .foregroundColor(SBTheme.accent200)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .overlay(
                    RoundedRectangle(cornerRadius: 3)
                        .stroke(SBTheme.accent300, lineWidth: 1)
                )
        } else {
            Text(key.state.uppercased())
                .font(.system(size: 9.5, weight: .regular))
                .tracking(0.9)
                .foregroundColor(SBTheme.neutral400)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .overlay(
                    RoundedRectangle(cornerRadius: 3)
                        .stroke(SBTheme.borderBadge, lineWidth: 1)
                )
        }
    }

    private func usageBar(label: String, percent: Double, isExhausted: Bool) -> some View {
        HStack(spacing: 10) {
            Text(label)
                .font(.system(size: 9.5, weight: .regular))
                .tracking(1.0)
                .foregroundColor(SBTheme.neutral600)
                .frame(width: 58, alignment: .leading)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Rectangle()
                        .fill(SBTheme.trackBg)
                        .frame(height: 3)

                    Rectangle()
                        .fill(barColor(percent: percent, isExhausted: isExhausted))
                        .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))), height: 3)
                }
            }
            .frame(height: 3)

            Text(Formatters.percent(percent))
                .font(.system(size: 11.5))
                .monospacedDigit()
                .foregroundColor(percentTextColor(percent: percent, isExhausted: isExhausted))
                .frame(width: 40, alignment: .trailing)
        }
    }

    private func barColor(percent: Double, isExhausted: Bool) -> Color {
        if isExhausted || percent >= 95 {
            return SBTheme.accent300
        }
        if percent >= 70 {
            return SBTheme.accent500
        }
        return SBTheme.neutral400
    }

    private func percentTextColor(percent: Double, isExhausted: Bool) -> Color {
        if isExhausted || percent >= 95 {
            return SBTheme.accent200
        }
        if percent >= 70 {
            return SBTheme.accent300
        }
        return SBTheme.neutral300
    }

    private var footerText: String {
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
}

// MARK: - Stats Section
struct StatsSection: View {
    let snapshot: MetricsSnapshot?

    var body: some View {
        if let snap = snapshot {
            let exhaustions = snap.keyExhaustions.reduce(0) { $0 + $1.count }
            let switches = snap.keySwitches.reduce(0) { $0 + $1.count }

            HStack(spacing: 18) {
                Label("\(snap.activeSessions) session\(snap.activeSessions == 1 ? "" : "s")", systemImage: "person.2")
                    .foregroundColor(SBTheme.neutral500)

                Label("\(exhaustions) exhaustion\(exhaustions == 1 ? "" : "s")", systemImage: "bolt.slash")
                    .foregroundColor(exhaustions > 0 ? SBTheme.accent400 : SBTheme.neutral500)

                Label("\(switches) switch\(switches == 1 ? "" : "es")", systemImage: "arrow.left.arrow.right")
                    .foregroundColor(SBTheme.neutral500)
            }
            .font(.system(size: 11))
            .monospacedDigit()
        }
    }
}

// MARK: - Skeleton Loading View
struct SkeletonLoadingView: View {
    var body: some View {
        VStack(spacing: 9) {
            skeletonCard
            skeletonCard
        }
    }

    private var skeletonCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Rectangle()
                    .fill(Color.white.opacity(0.09))
                    .frame(width: 120, height: 11)
                Spacer()
                Rectangle()
                    .fill(Color.white.opacity(0.07))
                    .frame(width: 56, height: 11)
            }
            VStack(spacing: 7) {
                skeletonRow
                skeletonRow
                skeletonRow
            }
        }
        .padding(EdgeInsets(top: 11, leading: 12, bottom: 10, trailing: 12))
        .background(Color.clear)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(Color.white.opacity(0.09), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 4))
    }

    private var skeletonRow: some View {
        HStack(spacing: 10) {
            Rectangle()
                .fill(Color.white.opacity(0.07))
                .frame(width: 58, height: 6)
            Rectangle()
                .fill(Color.white.opacity(0.09))
                .frame(height: 3)
            Rectangle()
                .fill(Color.white.opacity(0.07))
                .frame(width: 40, height: 6)
        }
    }
}

// MARK: - Overflow Menu Dropdown
struct OverflowMenuView: View {
    @ObservedObject var model: StatusModel
    @Binding var isPresented: Bool
    @State private var hoveredItem: String? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            menuItem("Validate keys") {
                Task {
                    await model.perform(.validateKeys)
                    isPresented = false
                }
            }

            menuItem("Reload config") {
                Task {
                    await model.perform(.reloadConfig)
                    isPresented = false
                }
            }

            Rectangle()
                .fill(SBTheme.borderMuted)
                .frame(height: 1)
                .padding(.vertical, 5)

            menuItem("Reset all quotas", isAccent: true) {
                Task {
                    await model.perform(.resetAllKeys)
                    isPresented = false
                }
            }
        }
        .padding(.vertical, 5)
        .frame(minWidth: 178)
        .background(SBTheme.overflowBg)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(SBTheme.borderOverflow, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 4))
        .shadow(color: Color.black.opacity(0.6), radius: 17, x: 0, y: 7)
    }

    private func menuItem(_ text: String, isAccent: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack {
                Text(text)
                    .font(.system(size: 12.5))
                    .foregroundColor(isAccent ? SBTheme.accent300 : SBTheme.neutral300)
                Spacer()
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(hoveredItem == text ? SBTheme.accentHover : Color.clear)
        }
        .buttonStyle(.plain)
        .onHover { isHovered in
            if isHovered {
                hoveredItem = text
            } else if hoveredItem == text {
                hoveredItem = nil
            }
        }
    }
}

// MARK: - Main Popover View
public struct PopoverContentView: View {
    @EnvironmentObject var model: StatusModel
    @State private var showSettings = false
    @State private var showOverflowMenu = false
    @State private var spinAngle: Double = 0
    @State private var isHoveringRefresh = false
    @State private var isHoveringSettings = false
    @State private var isHoveringOverflow = false
    @State private var isHoveringDashboard = false
    @State private var isHoveringQuit = false
    @State private var isHoveringReloadEmpty = false

    public init() {}

    public var body: some View {
        ZStack(alignment: .topTrailing) {
            VStack(alignment: .leading, spacing: 0) {
                headerView

                statusSummaryLine
                    .padding(.top, 9)

                if let bannerText = alertBannerText {
                    alertBanner(text: bannerText)
                        .padding(.top, 12)
                }

                Rectangle()
                    .fill(SBTheme.borderMuted)
                    .frame(height: 1)
                    .padding(.vertical, 12)

                if showSettings {
                    SettingsSection()
                } else {
                    mainBodySection
                }

                Rectangle()
                    .fill(SBTheme.borderMuted)
                    .frame(height: 1)
                    .padding(.top, 12)
                    .padding(.bottom, 11)

                footerView
            }
            .padding(EdgeInsets(top: 14, leading: 15, bottom: 13, trailing: 15))
            .background(SBTheme.bg)

            // Overflow dropdown positioned right below action buttons
            if showOverflowMenu {
                Color.black.opacity(0.001)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .onTapGesture {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            showOverflowMenu = false
                        }
                    }

                OverflowMenuView(model: model, isPresented: $showOverflowMenu)
                    .padding(.top, 44)
                    .padding(.trailing, 15)
                    .transition(.opacity.combined(with: .scale(scale: 0.96, anchor: .topTrailing)))
            }
        }
        .frame(width: 400)
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(SBTheme.borderOuter, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .preferredColorScheme(.dark)
        .task {
            await model.refresh()
        }
        .onChange(of: model.isRefreshing) { isRefreshing in
            if isRefreshing {
                withAnimation(.linear(duration: 1.1).repeatForever(autoreverses: false)) {
                    spinAngle = 360
                }
            } else {
                withAnimation(.default) {
                    spinAngle = 0
                }
            }
        }
    }

    // MARK: - Header Bar
    private var headerView: some View {
        HStack(alignment: .center, spacing: 10) {
            SwitchboardMarkView(size: 22)

            Text("Switchboard Go")
                .font(.sbHeading(size: 19, weight: .regular))
                .foregroundColor(SBTheme.neutral100)
                .tracking(-0.2)

            Spacer()

            HStack(spacing: 6) {
                // Refresh Button
                Button {
                    Task {
                        await model.refresh(force: true)
                    }
                } label: {
                    Image(systemName: "arrow.clockwise")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(model.isRefreshing || isHoveringRefresh ? SBTheme.accent300 : SBTheme.neutral400)
                        .rotationEffect(.degrees(spinAngle))
                        .frame(width: 26, height: 26)
                        .background(isHoveringRefresh || model.isRefreshing ? SBTheme.accentHover : Color.clear)
                        .overlay(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(isHoveringRefresh || model.isRefreshing ? SBTheme.accent600 : SBTheme.borderSubtle, lineWidth: 1)
                        )
                        .cornerRadius(3)
                }
                .buttonStyle(.plain)
                .onHover { isHoveringRefresh = $0 }
                .help("Refresh")

                // Settings Button
                Button {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        showSettings.toggle()
                        if showSettings {
                            showOverflowMenu = false
                        }
                    }
                } label: {
                    Image(systemName: "slider.horizontal.3")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(showSettings || isHoveringSettings ? SBTheme.accent300 : SBTheme.neutral400)
                        .frame(width: 26, height: 26)
                        .background(showSettings || isHoveringSettings ? SBTheme.accentHover : Color.clear)
                        .overlay(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(showSettings || isHoveringSettings ? SBTheme.accent600 : SBTheme.borderSubtle, lineWidth: 1)
                        )
                        .cornerRadius(3)
                }
                .buttonStyle(.plain)
                .onHover { isHoveringSettings = $0 }
                .help("Settings")

                // More Actions Button
                Button {
                    withAnimation(.easeInOut(duration: 0.15)) {
                        showOverflowMenu.toggle()
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(showOverflowMenu || isHoveringOverflow ? SBTheme.accent300 : SBTheme.neutral400)
                        .frame(width: 26, height: 26)
                        .background(showOverflowMenu || isHoveringOverflow ? SBTheme.accentHover : Color.clear)
                        .overlay(
                            RoundedRectangle(cornerRadius: 3)
                                .stroke(showOverflowMenu || isHoveringOverflow ? SBTheme.accent600 : SBTheme.borderSubtle, lineWidth: 1)
                        )
                        .cornerRadius(3)
                }
                .buttonStyle(.plain)
                .onHover { isHoveringOverflow = $0 }
                .help("More actions")
            }
        }
    }

    // MARK: - Status Summary Line
    private var statusSummaryLine: some View {
        Group {
            if let usage = model.usage {
                if usage.keys.isEmpty {
                    Text("0 OF 0 KEYS AVAILABLE")
                        .font(.system(size: 11, weight: .regular))
                        .tracking(1.1)
                        .monospacedDigit()
                        .foregroundColor(SBTheme.neutral500)
                } else {
                    let available = usage.summary.availableKeys
                    let total = usage.summary.totalKeys
                    let percent = Int(usage.rolling.percent.rounded())
                    Text("\(available) OF \(total) KEYS AVAILABLE · ROLLING \(percent)%")
                        .font(.system(size: 11, weight: .regular))
                        .tracking(1.1)
                        .monospacedDigit()
                        .foregroundColor(SBTheme.neutral500)
                }
            } else {
                Text("CONTACTING PROXY…")
                    .font(.system(size: 11, weight: .regular))
                    .tracking(1.1)
                    .foregroundColor(SBTheme.neutral600)
            }
        }
    }

    // MARK: - Alert Banner
    private var alertBannerText: String? {
        if let err = model.lastError {
            return err
        }
        if let usage = model.usage {
            if let exhaustedKey = usage.keys.first(where: { $0.state == "exhausted" }) {
                let currentKey = usage.keys.first(where: { $0.current })
                let retryStr = exhaustedKey.retryAfterSeconds != nil ? "; retry in \(exhaustedKey.retryAfterSeconds!)s" : ""
                let targetHint = currentKey != nil ? "Traffic moved to key \(currentKey!.index)" : "All keys exhausted"
                return "Key \(exhaustedKey.index) exhausted its 5-hour window. \(targetHint)\(retryStr)."
            }
        }
        return nil
    }

    private func alertBanner(text: String) -> some View {
        HStack(alignment: .top, spacing: 9) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 13))
                .foregroundColor(SBTheme.accent300)
                .padding(.top, 1)

            Text(text)
                .font(.system(size: 12))
                .lineSpacing(3)
                .foregroundColor(SBTheme.neutral300)

            Spacer()
        }
        .padding(EdgeInsets(top: 9, leading: 11, bottom: 9, trailing: 11))
        .background(SBTheme.accentBannerBg)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(SBTheme.accent500, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 4))
    }

    // MARK: - Main Body Section
    @ViewBuilder
    private var mainBodySection: some View {
        if let usage = model.usage {
            if usage.keys.isEmpty {
                emptyKeysCard
            } else {
                VStack(spacing: 9) {
                    ForEach(usage.keys) { key in
                        KeyRowView(key: key)
                    }
                }

                StatsSection(snapshot: model.snapshot)
                    .padding(.top, 12)
            }
        } else {
            SkeletonLoadingView()
        }
    }

    // MARK: - Empty Keys Card
    private var emptyKeysCard: some View {
        VStack(spacing: 7) {
            Text("No keys configured")
                .font(.sbHeading(size: 17))
                .foregroundColor(SBTheme.neutral200)

            Text("Add API keys to config.yaml, then reload the config from the actions menu.")
                .font(.system(size: 12))
                .lineSpacing(3)
                .foregroundColor(SBTheme.neutral500)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 270)

            Button {
                Task {
                    await model.perform(.reloadConfig)
                }
            } label: {
                Text("Reload config")
                    .font(.sbHeading(size: 12.5, weight: .semibold))
                    .foregroundColor(SBTheme.accent300)
                    .padding(.horizontal, 13)
                    .padding(.vertical, 6)
                    .background(isHoveringReloadEmpty ? SBTheme.accentHover : Color.clear)
                    .overlay(
                        RoundedRectangle(cornerRadius: 3)
                            .stroke(isHoveringReloadEmpty ? SBTheme.accent400 : SBTheme.accent600, lineWidth: 1)
                    )
                    .cornerRadius(3)
            }
            .buttonStyle(.plain)
            .padding(.top, 7)
            .onHover { isHoveringReloadEmpty = $0 }
        }
        .padding(.vertical, 22)
        .padding(.horizontal, 18)
        .frame(maxWidth: .infinity)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(SBTheme.borderMuted, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 4))
    }

    // MARK: - Footer Bar
    private var footerView: some View {
        HStack(alignment: .center, spacing: 8) {
            // Open Dashboard Button
            Button {
                if let base = (model.api as? APIClient)?.baseURL {
                    let url = base.appendingPathComponent("dashboard", isDirectory: true)
                    NSWorkspace.shared.open(url)
                }
            } label: {
                let isDisabled = model.usage == nil
                Text("Open Dashboard")
                    .font(.sbHeading(size: 12.5, weight: .semibold))
                    .foregroundColor(isDisabled ? SBTheme.neutral600 : SBTheme.accent300)
                    .padding(.horizontal, 13)
                    .padding(.vertical, 6)
                    .background(!isDisabled && isHoveringDashboard ? SBTheme.accentHover : Color.clear)
                    .overlay(
                        RoundedRectangle(cornerRadius: 3)
                            .stroke(isDisabled ? Color.white.opacity(0.14) : (isHoveringDashboard ? SBTheme.accent400 : SBTheme.accent600), lineWidth: 1)
                    )
                    .cornerRadius(3)
            }
            .buttonStyle(.plain)
            .disabled(model.usage == nil)
            .onHover { isHoveringDashboard = $0 }

            // Polling and Host info
            Text(connectionFooterText)
                .font(.system(size: 10.5))
                .monospacedDigit()
                .foregroundColor(SBTheme.neutral600)

            Spacer()

            // Quit Button
            Button {
                NSApplication.shared.terminate(nil)
            } label: {
                Text("Quit")
                    .font(.sbHeading(size: 12.5, weight: .semibold))
                    .foregroundColor(isHoveringQuit ? SBTheme.neutral200 : SBTheme.neutral400)
                    .padding(.horizontal, 13)
                    .padding(.vertical, 6)
                    .background(isHoveringQuit ? Color.white.opacity(0.07) : Color.clear)
                    .overlay(
                        RoundedRectangle(cornerRadius: 3)
                            .stroke(SBTheme.borderSubtle, lineWidth: 1)
                    )
                    .cornerRadius(3)
            }
            .buttonStyle(.plain)
            .onHover { isHoveringQuit = $0 }
        }
    }

    private var connectionFooterText: String {
        let hostPort: String
        if let base = (model.api as? APIClient)?.baseURL {
            hostPort = "\(base.host ?? "127.0.0.1")\(base.port != nil ? ":\(base.port!)" : "")"
        } else {
            hostPort = "127.0.0.1:8495"
        }
        return "\(hostPort) · polling \(Int(model.pollInterval))s"
    }
}
