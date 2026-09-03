import SwiftUI

public enum SBTheme {
    // MARK: - Surface Colors
    public static let bg = Color(hex: 0x191817)
    public static let overflowBg = Color(hex: 0x232120)
    public static let accentHover = Color(hex: 0xB68235).opacity(0.14)
    public static let accentBannerBg = Color(hex: 0xB68235).opacity(0.09)
    public static let accentToggleBg = Color(hex: 0xB68235).opacity(0.16)

    // MARK: - Neutrals
    public static let neutral100 = Color(hex: 0xF8F4F4)
    public static let neutral200 = Color(hex: 0xEAE7E7)
    public static let neutral300 = Color(hex: 0xD7D3D3)
    public static let neutral400 = Color(hex: 0xBAB6B6)
    public static let neutral500 = Color(hex: 0x9B9797)
    public static let neutral600 = Color(hex: 0x7D7979)
    public static let neutral700 = Color(hex: 0x605D5D)
    public static let neutral800 = Color(hex: 0x444141)
    public static let neutral900 = Color(hex: 0x2D2B2B)

    // MARK: - Accents
    public static let accent100 = Color(hex: 0xFFF3E4)
    public static let accent200 = Color(hex: 0xFFE3BF)
    public static let accent300 = Color(hex: 0xFACB8D)
    public static let accent400 = Color(hex: 0xE1AD66)
    public static let accent500 = Color(hex: 0xC28D41)
    public static let accent600 = Color(hex: 0xA06F24)
    public static let brandGold = Color(hex: 0xB68235)

    // MARK: - Borders & Tracks
    public static let borderOuter = Color(hex: 0xF3F2F2).opacity(0.11)
    public static let borderMuted = Color(hex: 0xF3F2F2).opacity(0.13)
    public static let borderSubtle = Color(hex: 0xF3F2F2).opacity(0.18)
    public static let borderBadge = Color(hex: 0xF3F2F2).opacity(0.26)
    public static let borderOverflow = Color(hex: 0xF3F2F2).opacity(0.16)
    public static let trackBg = Color(hex: 0xF3F2F2).opacity(0.14)
}

public extension Color {
    init(hex: UInt32, alpha: Double = 1.0) {
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        self.init(.sRGB, red: r, green: g, blue: b, opacity: alpha)
    }
}

public extension Font {
    static func sbHeading(size: CGFloat, weight: Font.Weight = .regular) -> Font {
        if NSFont(name: "Cormorant Garamond", size: size) != nil {
            return .custom("Cormorant Garamond", size: size)
        }
        return .system(size: size, weight: weight, design: .serif)
    }

    static func sbBody(size: CGFloat, weight: Font.Weight = .regular) -> Font {
        if NSFont(name: "Lora", size: size) != nil {
            return .custom("Lora", size: size)
        }
        return .system(size: size, weight: weight, design: .default)
    }
}
