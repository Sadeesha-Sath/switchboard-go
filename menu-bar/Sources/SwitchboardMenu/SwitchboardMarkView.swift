import SwiftUI

public struct SwitchboardMarkView: View {
    public var size: CGFloat = 22
    public var tint: Color = SBTheme.brandGold

    public init(size: CGFloat = 22, tint: Color = SBTheme.brandGold) {
        self.size = size
        self.tint = tint
    }

    public var body: some View {
        Canvas { context, canvasSize in
            let scale = min(canvasSize.width, canvasSize.height) / 40.0
            context.translateBy(x: (canvasSize.width - 40.0 * scale) / 2, y: (canvasSize.height - 40.0 * scale) / 2)
            context.scaleBy(x: scale, y: scale)

            // Outer ring arc (dasharray 82, 28 rotated by -58 deg)
            var outerArc = Path()
            outerArc.addArc(
                center: CGPoint(x: 20, y: 20),
                radius: 17.5,
                startAngle: .degrees(-58),
                endAngle: .degrees(-58 + (82.0 / 109.9557) * 360.0),
                clockwise: false
            )
            context.stroke(
                outerArc,
                with: .color(tint.opacity(0.85)),
                style: StrokeStyle(lineWidth: 1.5, lineCap: .round)
            )

            // Inner hairpin bridge
            var bridge = Path()
            bridge.move(to: CGPoint(x: 12.5, y: 25.5))
            bridge.addCurve(
                to: CGPoint(x: 27.5, y: 25.5),
                control1: CGPoint(x: 12.5, y: 14.5),
                control2: CGPoint(x: 27.5, y: 14.5)
            )
            context.stroke(
                bridge,
                with: .color(tint),
                style: StrokeStyle(lineWidth: 1.6, lineCap: .round)
            )

            // Left terminal circle
            let leftCircle = Path(ellipseIn: CGRect(x: 12.5 - 1.9, y: 26.5 - 1.9, width: 3.8, height: 3.8))
            context.stroke(
                leftCircle,
                with: .color(tint),
                style: StrokeStyle(lineWidth: 1.5)
            )

            // Right terminal circle
            let rightCircle = Path(ellipseIn: CGRect(x: 27.5 - 1.9, y: 26.5 - 1.9, width: 3.8, height: 3.8))
            context.stroke(
                rightCircle,
                with: .color(tint),
                style: StrokeStyle(lineWidth: 1.5)
            )

            // Center vertical tick
            var tick = Path()
            tick.move(to: CGPoint(x: 20, y: 24.6))
            tick.addLine(to: CGPoint(x: 20, y: 28.4))
            context.stroke(
                tick,
                with: .color(tint),
                style: StrokeStyle(lineWidth: 1.5, lineCap: .round)
            )

            // Center filled node
            let centerDot = Path(ellipseIn: CGRect(x: 20 - 1.5, y: 26.5 - 1.5, width: 3.0, height: 3.0))
            context.fill(centerDot, with: .color(tint))
        }
        .frame(width: size, height: size)
    }
}
