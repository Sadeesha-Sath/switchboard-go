import Foundation

public enum APIError: LocalizedError, Equatable {
    case unauthorized
    case http(Int)
    case transport(String)

    public var errorDescription: String? {
        switch self {
        case .unauthorized:
            return "Invalid proxy API key (401)"
        case .http(let status):
            return "Proxy returned HTTP \(status)"
        case .transport(let message):
            return "Cannot reach proxy: \(message)"
        }
    }
}

public protocol APIServing {
    func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage
    func fetchStatus() async throws -> StatusResponse
    func fetchMetrics() async throws -> MetricsSnapshot
    func validateKeys() async throws
    func resetKey(index: Int) async throws
    func resetAllKeys() async throws
    func reloadConfig() async throws
}

public final class APIClient: APIServing {
    public var baseURL: URL
    public var apiKey: String
    private let session: URLSession

    public init(baseURL: URL, apiKey: String,
                session: URLSession = URLSession(configuration: .ephemeral)) {
        self.baseURL = baseURL
        self.apiKey = apiKey
        self.session = session
    }

    public func fetchUsage(forceRefresh: Bool) async throws -> AggregatedUsage {
        try await get(path: forceRefresh ? "/usage?refresh=true" : "/usage")
    }

    public func fetchStatus() async throws -> StatusResponse {
        try await get(path: "/admin/status")
    }

    public func fetchMetrics() async throws -> MetricsSnapshot {
        try await get(path: "/dashboard/api/metrics.json")
    }

    public func validateKeys() async throws {
        try await post(path: "/admin/validate-keys")
    }

    public func resetKey(index: Int) async throws {
        try await post(path: "/admin/reset-key", body: ["index": index])
    }

    public func resetAllKeys() async throws {
        try await post(path: "/admin/reset-all-keys")
    }

    public func reloadConfig() async throws {
        try await post(path: "/admin/reload")
    }

    private func get<T: Decodable>(path: String) async throws -> T {
        let (data, response) = try await run(request(path: path))
        return try decode(data, response)
    }

    private func post(path: String, body: [String: Any]? = nil) async throws {
        var request = request(path: path)
        request.httpMethod = "POST"
        if let body {
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (_, response) = try await run(request)
        try validate(response)
    }

    private func request(path: String) -> URLRequest {
        var base = baseURL.absoluteString
        while base.hasSuffix("/") { base.removeLast() }
        var request = URLRequest(url: URL(string: base + path)!)
        request.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        request.timeoutInterval = 10
        return request
    }

    private func validate(_ response: URLResponse) throws {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport("non-HTTP response")
        }
        switch http.statusCode {
        case 401:
            throw APIError.unauthorized
        case 200..<300:
            return
        default:
            throw APIError.http(http.statusCode)
        }
    }

    private func run(_ request: URLRequest) async throws -> (Data, URLResponse) {
        do {
            return try await session.data(for: request)
        } catch {
            throw APIError.transport(error.localizedDescription)
        }
    }

    private func decode<T: Decodable>(_ data: Data, _ response: URLResponse) throws -> T {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport("non-HTTP response")
        }
        switch http.statusCode {
        case 401:
            throw APIError.unauthorized
        case 200..<300:
            return try JSONDecoder().decode(T.self, from: data)
        default:
            throw APIError.http(http.statusCode)
        }
    }
}
