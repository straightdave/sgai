import SwiftUI

struct ContentView: View {
    @StateObject private var playerController = InterstitialPlayerController()

    @State private var serverURL = "http://localhost:8080"
    @State private var mode: DemoMode = .vod
    @State private var healthText = "Checking API..."
    @State private var healthOK: Bool?

    @State private var liveStatus = LiveStatus(running: false, masterUrl: nil, startedAt: nil, elapsedSeconds: nil)
    @State private var liveBusy = false
    @State private var liveError: String?

    private let vodPath = "/priv/big_buck_bunny/x36xhzz.m3u8"

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    serverConfigCard

                    Picker("Demo Mode", selection: $mode) {
                        ForEach(DemoMode.allCases) { item in
                            Text(item.rawValue).tag(item)
                        }
                    }
                    .pickerStyle(.segmented)

                    if mode == .live {
                        liveControlsCard
                    }

                    InterstitialPlayerView(
                        controller: playerController,
                        emptyMessage: mode == .live
                            ? "Live stream is offline. Tap \"Start Live Stream\" above to generate and play the endless HLS stream."
                            : "Tap Connect to load the VOD stream."
                    )

                    if let sessionID = playerController.sessionID {
                        Text("Session ID: \(sessionID)")
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }

                    if let error = playerController.playerError {
                        Text(error)
                            .font(.footnote)
                            .foregroundStyle(.red)
                    }

                    healthBadge
                }
                .padding()
            }
            .navigationTitle("SGAI iOS Demo")
            .navigationBarTitleDisplayMode(.inline)
            .task {
                await refreshHealthAndLoad()
            }
            .onChange(of: mode) { _, newMode in
                Task {
                    await handleModeChange(newMode)
                }
            }
        }
    }

    private var serverConfigCard: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Go Server URL")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                TextField("http://localhost:8080", text: $serverURL)
                    .textFieldStyle(.roundedBorder)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                Button("Connect") {
                    Task {
                        await refreshHealthAndLoad()
                    }
                }
                .buttonStyle(.borderedProminent)
            }
            Text("Use localhost:8080 in Simulator, or your Mac's LAN IP on a physical iPhone.")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .padding(12)
        .background(Color(.secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    private var liveControlsCard: some View {
        VStack(spacing: 10) {
            HStack(spacing: 10) {
                Button {
                    Task { await startLive() }
                } label: {
                    Text(liveBusy && !liveStatus.running ? "Starting..." : "Start Live Stream")
                }
                .buttonStyle(.borderedProminent)
                .disabled(liveBusy || liveStatus.running)

                Button {
                    Task { await stopLive() }
                } label: {
                    Text("Stop Live Stream")
                }
                .buttonStyle(.bordered)
                .disabled(liveBusy || !liveStatus.running)

                Spacer()

                Text(liveStatus.running ? "LIVE" : "OFFLINE")
                    .font(.caption2.weight(.bold))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5)
                    .background(liveStatus.running ? Color.red.opacity(0.15) : Color.gray.opacity(0.2))
                    .foregroundStyle(liveStatus.running ? .red : .secondary)
                    .clipShape(Capsule())
            }

            if let liveError {
                Text(liveError)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .padding(12)
        .background(Color(.secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    private var healthBadge: some View {
        Text(healthText)
            .font(.caption.weight(.medium))
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(
                healthOK == true
                    ? Color.green.opacity(0.15)
                    : (healthOK == false ? Color.red.opacity(0.15) : Color.gray.opacity(0.15))
            )
            .foregroundStyle(
                healthOK == true ? .green : (healthOK == false ? .red : .secondary)
            )
            .clipShape(Capsule())
    }

    private var normalizedBaseURL: String {
        serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }

    private func refreshHealthAndLoad() async {
        await checkHealth()
        await handleModeChange(mode)
    }

    private func checkHealth() async {
        guard let url = URL(string: "\(normalizedBaseURL)/api/health") else {
            healthOK = false
            healthText = "Invalid server URL"
            return
        }
        do {
            let (data, response) = try await URLSession.shared.data(from: url)
            if let http = response as? HTTPURLResponse, !(200...299).contains(http.statusCode) {
                throw URLError(.badServerResponse)
            }
            let decoded = try JSONDecoder().decode(HealthResponse.self, from: data)
            healthOK = true
            healthText = "API status: \(decoded.status)"
        } catch {
            healthOK = false
            healthText = "API error: \(error.localizedDescription)"
        }
    }

    private func handleModeChange(_ newMode: DemoMode) async {
        liveError = nil
        if newMode == .vod {
            playerController.load(streamPath: vodPath, baseURL: normalizedBaseURL, isLive: false)
            return
        }

        playerController.stop()
        await fetchLiveStatus()
        if liveStatus.running, let masterUrl = liveStatus.masterUrl {
            playerController.load(streamPath: masterUrl, baseURL: normalizedBaseURL, isLive: true)
        }
    }

    private func fetchLiveStatus() async {
        guard let url = URL(string: "\(normalizedBaseURL)/api/live/status") else { return }
        do {
            let (data, _) = try await URLSession.shared.data(from: url)
            liveStatus = try JSONDecoder().decode(LiveStatus.self, from: data)
        } catch {
            liveError = "Failed to check live status: \(error.localizedDescription)"
        }
    }

    private func startLive() async {
        guard let url = URL(string: "\(normalizedBaseURL)/api/live/start") else { return }
        liveBusy = true
        liveError = nil
        defer { liveBusy = false }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            if let http = response as? HTTPURLResponse, !(200...299).contains(http.statusCode) {
                throw URLError(.badServerResponse)
            }
            liveStatus = try JSONDecoder().decode(LiveStatus.self, from: data)
            if liveStatus.running, let masterUrl = liveStatus.masterUrl {
                playerController.load(streamPath: masterUrl, baseURL: normalizedBaseURL, isLive: true)
            }
        } catch {
            liveError = "Start failed: \(error.localizedDescription)"
        }
    }

    private func stopLive() async {
        guard let url = URL(string: "\(normalizedBaseURL)/api/live/stop") else { return }
        liveBusy = true
        liveError = nil
        defer { liveBusy = false }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        do {
            let (data, _) = try await URLSession.shared.data(for: request)
            liveStatus = try JSONDecoder().decode(LiveStatus.self, from: data)
            playerController.stop()
        } catch {
            liveError = "Stop failed: \(error.localizedDescription)"
        }
    }
}
