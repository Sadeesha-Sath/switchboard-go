import XCTest
@testable import SwitchboardMenu

final class MockURLProtocol: URLProtocol {
    nonisolated(unsafe) static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?
    /// URLProtocol does not expose request.httpBody — it arrives as a stream.
    /// startLoading() drains it here so tests can assert on POST bodies.
    nonisolated(unsafe) static var lastBody: Data?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    private static func drainBody(_ request: URLRequest) -> Data? {
        if let data = request.httpBody { return data }
        guard let stream = request.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        let bufferSize = 4096
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
        defer { buffer.deallocate() }
        while stream.hasBytesAvailable {
            let read = stream.read(buffer, maxLength: bufferSize)
            if read <= 0 { break }
            data.append(buffer, count: read)
        }
        return data
    }

    override func startLoading() {
        Self.lastBody = Self.drainBody(request)
        guard let handler = MockURLProtocol.requestHandler else {
            client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

final class APIClientTests: XCTestCase {
    private func makeClient() -> APIClient {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [MockURLProtocol.self]
        return APIClient(baseURL: URL(string: "http://127.0.0.1:8495")!,
                         apiKey: "test-proxy-key",
                         session: URLSession(configuration: config))
    }

    func testFetchUsageAttachesBearerHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/usage")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-proxy-key")
            let body = Data("""
            {"rolling":{"status":"ok","percent":10},"weekly":{"status":"ok","percent":10},
             "monthly":{"status":"ok","percent":10},
             "summary":{"total_keys":1,"available_keys":1,"exhausted_keys":0,
                        "active_sessions":0,"routing_strategy":"session_sticky",
                        "proactive_threshold_percent":95.0},
             "keys":[]}
            """.utf8)
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, body)
        }
        let usage = try await makeClient().fetchUsage(forceRefresh: false)
        XCTAssertEqual(usage.summary.totalKeys, 1)
    }

    func testForceRefreshAppendsQuery() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.query, "refresh=true")
            let body = Data("""
            {"rolling":{"status":"ok","percent":0},"weekly":{"status":"ok","percent":0},
             "monthly":{"status":"ok","percent":0},
             "summary":{"total_keys":0,"available_keys":0,"exhausted_keys":0,
                        "active_sessions":0,"routing_strategy":"s","proactive_threshold_percent":95.0},
             "keys":[]}
            """.utf8)
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, body)
        }
        _ = try await makeClient().fetchUsage(forceRefresh: true)
    }

    func testUnauthorizedThrowsTypedError() async {
        MockURLProtocol.requestHandler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!, Data())
        }
        do {
            _ = try await makeClient().fetchUsage(forceRefresh: false)
            XCTFail("expected unauthorized")
        } catch {
            XCTAssertEqual(error as? APIError, .unauthorized)
        }
    }

    func testHTTPErrorIncludesStatus() async {
        MockURLProtocol.requestHandler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 503, httpVersion: nil, headerFields: nil)!, Data())
        }
        do {
            _ = try await makeClient().fetchUsage(forceRefresh: false)
            XCTFail("expected http error")
        } catch {
            XCTAssertEqual(error as? APIError, .http(503))
        }
    }

    func testResetKeyPostsJSONBody() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/admin/reset-key")
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data())
        }
        try await makeClient().resetKey(index: 0)
        let body = try JSONSerialization.jsonObject(with: MockURLProtocol.lastBody ?? Data()) as? [String: Int]
        XCTAssertEqual(body?["index"], 0)
    }

    func testReloadConfigPosts() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/admin/reload")
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data())
        }
        try await makeClient().reloadConfig()
    }

    func testReloadConfigUnauthorizedThrowsTypedError() async {
        MockURLProtocol.requestHandler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!, Data())
        }
        do {
            try await makeClient().reloadConfig()
            XCTFail("expected unauthorized")
        } catch {
            XCTAssertEqual(error as? APIError, .unauthorized)
        }
    }

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        MockURLProtocol.lastBody = nil
    }
}
