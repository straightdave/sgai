import AVFoundation
import AVKit
import SwiftUI

struct AVPlayerControllerView: UIViewControllerRepresentable {
    let player: AVPlayer
    let showsControls: Bool

    func makeUIViewController(context: Context) -> AVPlayerViewController {
        let controller = AVPlayerViewController()
        controller.player = player
        controller.showsPlaybackControls = showsControls
        controller.videoGravity = .resizeAspect
        return controller
    }

    func updateUIViewController(_ uiViewController: AVPlayerViewController, context: Context) {
        if uiViewController.player !== player {
            uiViewController.player = player
        }
        uiViewController.showsPlaybackControls = showsControls
    }
}

struct AdPlayerLayerView: UIViewRepresentable {
    let player: AVPlayer

    func makeUIView(context: Context) -> PlayerLayerUIView {
        let view = PlayerLayerUIView()
        view.playerLayer.player = player
        view.playerLayer.videoGravity = .resizeAspect
        return view
    }

    func updateUIView(_ uiView: PlayerLayerUIView, context: Context) {
        if uiView.playerLayer.player !== player {
            uiView.playerLayer.player = player
        }
    }

    final class PlayerLayerUIView: UIView {
        override class var layerClass: AnyClass { AVPlayerLayer.self }
        var playerLayer: AVPlayerLayer { layer as! AVPlayerLayer }
    }
}

struct InterstitialPlayerView: View {
    @ObservedObject var controller: InterstitialPlayerController
    let emptyMessage: String

    var body: some View {
        GeometryReader { geo in
            let isSqueezed = controller.adMode == .squeezeback
            let isFullscreenAd = controller.adMode == .fullscreen

            ZStack(alignment: .topTrailing) {
                Color.black

                // Ad player sits behind primary in squeezeback, or on top in fullscreen.
                AdPlayerLayerView(player: controller.adPlayer)
                    .frame(width: geo.size.width, height: geo.size.height)
                    .opacity(controller.adMode == nil ? 0 : 1)
                    .zIndex(isFullscreenAd ? 3 : 1)
                    .allowsHitTesting(false)

                // Primary player shrinks to top-right 75% during L-squeezeback.
                AVPlayerControllerView(
                    player: controller.primaryPlayer,
                    showsControls: controller.isLoaded && !isFullscreenAd
                )
                .frame(
                    width: isSqueezed ? geo.size.width * 0.75 : geo.size.width,
                    height: isSqueezed ? geo.size.height * 0.75 : geo.size.height
                )
                .animation(.easeInOut(duration: 0.25), value: isSqueezed)
                .zIndex(2)

                if !controller.isLoaded {
                    ZStack {
                        Color.black.opacity(0.9)
                        Text(emptyMessage)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)
                            .padding()
                    }
                    .frame(width: geo.size.width, height: geo.size.height)
                    .zIndex(4)
                }

                // Top-left Ad / Countdown badge
                VStack {
                    HStack {
                        if controller.adMode != nil {
                            badgeView("AD")
                        } else if let countdown = controller.adCountdown {
                            badgeView("AD IN \(countdown)S")
                        }
                        Spacer()
                    }
                    Spacer()
                }
                .padding(10)
                .zIndex(5)
                .allowsHitTesting(false)

                // Bottom-left CTA button
                if controller.adMode != nil, let button = controller.adButton {
                    VStack {
                        Spacer()
                        HStack {
                            Button {
                                controller.handleAdButtonTap()
                            } label: {
                                Text(controller.adButtonClicked ? "Clicked!" : button.label)
                                    .font(.caption.weight(.semibold))
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .background(Color.blue.opacity(0.92))
                                    .foregroundStyle(.white)
                                    .clipShape(RoundedRectangle(cornerRadius: 6))
                            }
                            Spacer()
                        }
                    }
                    .padding(10)
                    .zIndex(6)
                }
            }
        }
        .aspectRatio(16 / 9, contentMode: .fit)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    private func badgeView(_ text: String) -> some View {
        Text(text)
            .font(.caption2.weight(.bold))
            .tracking(0.6)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(Color.black.opacity(0.8))
            .foregroundStyle(.white)
            .clipShape(RoundedRectangle(cornerRadius: 4))
    }
}
