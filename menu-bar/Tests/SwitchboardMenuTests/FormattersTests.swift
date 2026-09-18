import XCTest
@testable import SwitchboardMenu

final class FormattersTests: XCTestCase {
    private let utc: TimeZone = TimeZone(secondsFromGMT: 0)!

    private func date(_ iso: String) -> Date {
        ISO8601DateFormatter.parse(iso)!
    }

    func testCountdownHoursMinutesUnchanged() {
        let now = date("2026-09-01T12:00:00Z")
        XCTAssertEqual(Formatters.countdown(from: "2026-09-01T14:05:00Z", now: now), "in 2h 5m")
        XCTAssertEqual(Formatters.countdown(from: "2026-09-01T12:45:00Z", now: now), "in 45m")
    }

    func testCountdownEmitsDaysForWeeklyScale() {
        let now = date("2026-09-01T12:00:00Z")
        // 3d 4h later
        XCTAssertEqual(Formatters.countdown(from: "2026-09-04T16:00:00Z", now: now), "in 3d 4h")
    }

    func testCountdownOmitsZeroHoursWhenExactDays() {
        let now = date("2026-09-01T12:00:00Z")
        // exactly 12d later
        XCTAssertEqual(Formatters.countdown(from: "2026-09-13T12:00:00Z", now: now), "in 12d")
    }

    func testKeyResetsFooterShowsAllThreeWindows() {
        let now = date("2026-09-01T12:00:00Z")
        let footer = Formatters.keyResetsFooter(
            rolling: "2026-09-01T14:05:00Z",
            weekly: "2026-09-04T16:00:00Z",
            monthly: "2026-09-13T12:00:00Z",
            error: nil,
            now: now
        )
        XCTAssertEqual(footer, "resets 5h in 2h 5m · weekly in 3d 4h · monthly in 12d")
    }

    func testKeyResetsFooterSkipsMissingAndAppendsError() {
        let now = date("2026-09-01T12:00:00Z")
        let footer = Formatters.keyResetsFooter(
            rolling: "2026-09-01T14:05:00Z",
            weekly: nil,
            monthly: "0001-01-01T00:00:00Z",
            error: "upstream /usage returned status 429",
            now: now
        )
        XCTAssertEqual(footer, "resets 5h in 2h 5m · upstream /usage returned status 429")
    }
}
