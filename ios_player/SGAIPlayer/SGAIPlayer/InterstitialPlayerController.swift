import AVFoundation
import Combine
import Foundation
import UIKit

@MainActor
final class InterstitialPlayerController: NSObject, ObservableObject {
    let primaryPlayer = AVPlayer()
    let adPlayer = AVPlayer()

    @Published var adMode: AdDisplayMode?
    @Published var adCountdown: Int?
    @Published var adButton: AdButton?
    @Published var adButtonClicked = false
    @Published var playerError: String?
    @Published var isLoaded = false
    @Published var sessionID: String?

    private var baseURLString = "http://localhost:8080"
    private var isLive = false
    private var breaks: [InterstitialBreak] = []
    private var triggeredIds = Set<String>()
    private var isLoadingAd = false
    private var wasMuted = false
    private var mutedByAd = false

    private var metadataCollector: AVPlayerItemMetadataCollector?
    private var timeObserverToken: Any?
    private var adEndObserver: NSObjectProtocol?
    private var adFailObserver: NSObjectProtocol?

    override init() {
        super.init()
        setupTimeObserver()
    }

    deinit {
        if let token = timeObserverToken {
            primaryPlayer.removeTimeObserver(token)
        }
        if let adEndObserver {
            NotificationCenter.default.removeObserver(adEndObserver)
        }
        if let adFailObserver {
            NotificationCenter.default.removeObserver(adFailObserver)
        }
    }

    func load(streamPath: String?, baseURL: String, isLive: Bool) {
        self.baseURLString = baseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        self.isLive = isLive

        resetState()

        let sid = UUID().uuidString
        guard let streamPath, !streamPath.isEmpty,
              let resolvedURL = resolveURL(streamPath),
              let url = appendSessionID(sid, to: resolvedURL) else {
            primaryPlayer.replaceCurrentItem(with: nil)
            sessionID = nil
            isLoaded = false
            return
        }

        sessionID = sid

        let item = AVPlayerItem(url: url)
        let collector = AVPlayerItemMetadataCollector()
        collector.setDelegate(self, queue: .main)
        item.add(collector)
        self.metadataCollector = collector

        primaryPlayer.replaceCurrentItem(with: item)
        primaryPlayer.play()
        isLoaded = true
    }

    func stop() {
        resetState()
        primaryPlayer.replaceCurrentItem(with: nil)
        sessionID = nil
        isLoaded = false
    }

    func handleAdButtonTap() {
        guard let button = adButton, let targetURL = resolveURL(button.url) else { return }

        if button.action == "request" {
            var request = URLRequest(url: targetURL)
            request.httpMethod = "POST"
            Task {
                do {
                    _ = try await URLSession.shared.data(for: request)
                    adButtonClicked = true
                } catch {
                    // Ignore transient click tracking errors in demo UI.
                }
            }
            return
        }

        UIApplication.shared.open(targetURL)
    }

    private func resetState() {
        finishAd(resumePrimary: false)
        breaks = []
        triggeredIds.removeAll()
        adCountdown = nil
        playerError = nil
        if let collector = metadataCollector {
            primaryPlayer.currentItem?.remove(collector)
            metadataCollector = nil
        }
    }

    private func setupTimeObserver() {
        let interval = CMTime(seconds: 0.25, preferredTimescale: CMTimeScale(NSEC_PER_SEC))
        timeObserverToken = primaryPlayer.addPeriodicTimeObserver(
            forInterval: interval,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.checkBreaks()
            }
        }
    }

    private func checkBreaks() {
        guard adMode == nil, !isLoadingAd,
              let currentDate = primaryPlayer.currentItem?.currentDate() else {
            return
        }

        if let nextBreak = breaks.first(where: {
            !triggeredIds.contains($0.id)
                && currentDate >= $0.startDate
                && currentDate.timeIntervalSince($0.startDate) < 2.0
        }) {
            triggeredIds.insert(nextBreak.id)
            adCountdown = nil
            Task {
                await startAd(for: nextBreak)
            }
            return
        }

        if let upcoming = breaks.first(where: {
            let delta = $0.startDate.timeIntervalSince(currentDate)
            return !triggeredIds.contains($0.id) && delta > 0 && delta <= 5.0
        }) {
            adCountdown = Int(ceil(upcoming.startDate.timeIntervalSince(currentDate)))
        } else {
            adCountdown = nil
        }
    }

    private func startAd(for item: InterstitialBreak) async {
        isLoadingAd = true
        adButton = nil
        adButtonClicked = false

        defer { isLoadingAd = false }

        do {
            var adURI = item.assetURI
            var layout = ""
            var buttonMeta: AdButton?

            if adURI == nil, let assetListPath = item.assetListPath,
               let listURL = buildAssetListURL(assetListPath) {
                let (data, response) = try await URLSession.shared.data(from: listURL)
                if let http = response as? HTTPURLResponse, !(200...299).contains(http.statusCode) {
                    throw URLError(.badServerResponse)
                }
                let decoded = try JSONDecoder().decode(AdAssetListResponse.self, from: data)
                if let first = decoded.assets.first {
                    adURI = first.uri
                    layout = first.layout ?? ""
                    buttonMeta = first.button
                }
            }

            guard let adURI, let resolvedAdURL = resolveURL(adURI) else {
                finishAd()
                return
            }

            let mode: AdDisplayMode = (layout == "l-squeezeback") ? .squeezeback : .fullscreen
            adMode = mode
            adButton = buttonMeta

            if mode == .fullscreen {
                mutePrimaryForAd()
                if !isLive {
                    primaryPlayer.pause()
                }
            }

            let asset = AVURLAsset(url: resolvedAdURL)
            if mode == .squeezeback {
                let audioTracks = (try? await asset.loadTracks(withMediaType: .audio)) ?? []
                if !audioTracks.isEmpty {
                    mutePrimaryForAd()
                }
            }

            let adItem = AVPlayerItem(asset: asset)
            observeAdCompletion(for: adItem)
            adPlayer.replaceCurrentItem(with: adItem)
            adPlayer.play()
        } catch {
            playerError = "Ad error: \(error.localizedDescription)"
            finishAd()
        }
    }

    private func observeAdCompletion(for item: AVPlayerItem) {
        if let adEndObserver {
            NotificationCenter.default.removeObserver(adEndObserver)
        }
        if let adFailObserver {
            NotificationCenter.default.removeObserver(adFailObserver)
        }

        adEndObserver = NotificationCenter.default.addObserver(
            forName: .AVPlayerItemDidPlayToEndTime,
            object: item,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.finishAd()
            }
        }

        adFailObserver = NotificationCenter.default.addObserver(
            forName: .AVPlayerItemFailedToPlayToEndTime,
            object: item,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.finishAd()
            }
        }
    }

    private func mutePrimaryForAd() {
        guard !mutedByAd else { return }
        wasMuted = primaryPlayer.isMuted
        primaryPlayer.isMuted = true
        mutedByAd = true
    }

    private func finishAd(resumePrimary: Bool = true) {
        let wasFullscreen = (adMode == .fullscreen)
        adPlayer.pause()
        adPlayer.replaceCurrentItem(with: nil)

        if let adEndObserver {
            NotificationCenter.default.removeObserver(adEndObserver)
            self.adEndObserver = nil
        }
        if let adFailObserver {
            NotificationCenter.default.removeObserver(adFailObserver)
            self.adFailObserver = nil
        }

        adMode = nil
        adButton = nil
        adButtonClicked = false

        if mutedByAd {
            primaryPlayer.isMuted = wasMuted
            mutedByAd = false
        }

        if resumePrimary && wasFullscreen && !isLive {
            primaryPlayer.play()
        }
    }

    private func resolveURL(_ raw: String) -> URL? {
        if raw.hasPrefix("http://") || raw.hasPrefix("https://") {
            return URL(string: raw)
        }
        let path = raw.hasPrefix("/") ? raw : "/\(raw)"
        return URL(string: "\(baseURLString)\(path)")
    }

    private func appendSessionID(_ sid: String, to url: URL) -> URL? {
        guard var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
            return nil
        }
        var items = (components.queryItems ?? []).filter { $0.name != "sid" }
        items.append(URLQueryItem(name: "sid", value: sid))
        components.queryItems = items
        return components.url
    }

    private func buildAssetListURL(_ raw: String) -> URL? {
        guard let url = resolveURL(raw),
              var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
            return nil
        }
        var items = components.queryItems ?? []
        items.append(URLQueryItem(name: "clickable", value: "1"))
        items.append(URLQueryItem(name: "squeezeback", value: "1"))
        components.queryItems = items
        return components.url
    }
}

extension InterstitialPlayerController: AVPlayerItemMetadataCollectorPushDelegate {
    nonisolated func metadataCollector(
        _ metadataCollector: AVPlayerItemMetadataCollector,
        didCollect metadataGroups: [AVDateRangeMetadataGroup],
        indexesOfNewGroups: IndexSet,
        indexesOfModifiedGroups: IndexSet
    ) {
        let parsed: [InterstitialBreak] = metadataGroups.compactMap { group in
            let isInterstitial = group.classifyingLabel == "com.apple.hls.interstitial"
                || group.items.contains {
                    ($0.key as? String) == "CLASS" && $0.stringValue == "com.apple.hls.interstitial"
                }
            guard isInterstitial else { return nil }

            let id = group.uniqueID
                ?? group.items.first(where: { ($0.key as? String) == "ID" })?.stringValue
                ?? UUID().uuidString

            let assetList = group.items.first {
                ($0.key as? String) == "X-ASSET-LIST"
            }?.stringValue

            let assetURI = group.items.first {
                ($0.key as? String) == "X-ASSET-URI"
            }?.stringValue

            return InterstitialBreak(
                id: id,
                startDate: group.startDate,
                assetListPath: assetList,
                assetURI: assetURI
            )
        }
        .sorted { $0.startDate < $1.startDate }

        Task { @MainActor [weak self] in
            self?.breaks = parsed
        }
    }
}
