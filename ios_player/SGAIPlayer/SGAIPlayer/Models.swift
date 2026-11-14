import Foundation

enum DemoMode: String, CaseIterable, Identifiable {
    case vod = "VOD Demo"
    case live = "Linear (Live) Demo"

    var id: String { rawValue }
}

enum AdDisplayMode: Equatable {
    case fullscreen
    case squeezeback
}

struct HealthResponse: Decodable {
    let status: String
}

struct LiveStatus: Decodable {
    let running: Bool
    let masterUrl: String?
    let startedAt: String?
    let elapsedSeconds: Double?
}

struct AdButton: Decodable, Equatable {
    let label: String
    let action: String
    let url: String

    enum CodingKeys: String, CodingKey {
        case label = "LABEL"
        case action = "ACTION"
        case url = "URL"
    }
}

struct AdAsset: Decodable {
    let uri: String
    let duration: Double
    let layout: String?
    let button: AdButton?

    enum CodingKeys: String, CodingKey {
        case uri = "URI"
        case duration = "DURATION"
        case layout = "LAYOUT"
        case button = "BUTTON"
    }
}

struct AdAssetListResponse: Decodable {
    let assets: [AdAsset]

    enum CodingKeys: String, CodingKey {
        case assets = "ASSETS"
    }
}

struct InterstitialBreak: Identifiable {
    let id: String
    let startDate: Date
    let assetListPath: String?
    let assetURI: String?
}
