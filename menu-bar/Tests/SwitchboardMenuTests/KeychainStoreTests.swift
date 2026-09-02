import XCTest
@testable import SwitchboardMenu

final class KeychainStoreTests: XCTestCase {
    func testSaveLoadDeleteRoundTrip() {
        KeychainStore.delete()
        XCTAssertNil(KeychainStore.load())

        KeychainStore.save("sk-test-key-abc")
        XCTAssertEqual(KeychainStore.load(), "sk-test-key-abc")

        KeychainStore.save("sk-test-key-updated")
        XCTAssertEqual(KeychainStore.load(), "sk-test-key-updated")

        KeychainStore.delete()
        XCTAssertNil(KeychainStore.load())
    }
}
