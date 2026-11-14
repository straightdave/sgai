# SGAI iOS Player Demo

Native SwiftUI + AVFoundation demo client that connects to the Go SGAI server for both **VOD** and **Linear (Live)** HLS interstitials.

## Features

- **Server URL bar** (`http://localhost:8080` for iOS Simulator, or your Mac's LAN IP for a physical iPhone) with `/api/health` check.
- **VOD Demo & Linear (Live) Demo** mode switcher.
- **Live stream controls** (`POST /api/live/start`, `POST /api/live/stop`, `GET /api/live/status`).
- **HLS Interstitials (`#EXT-X-DATERANGE`)** via `AVPlayerItemMetadataCollector` and `AVPlayerItem.currentDate()`:
  - Requests `/api/ads?...&clickable=1&squeezeback=1`.
  - Supports **fullscreen** ads (pauses VOD; keeps live stream advancing muted at the live edge) and **L-squeezeback** ads (shrinks primary video to top-right 75%).
  - **Audio-aware muting**: mutes primary video during squeezeback only if the ad asset has an audio track.
  - **Interactive CTA button** (`link` opens URL, `request` sends `POST` to `/api/ads/click`).

## Running

1. Start the Go server from the repo root:

   ```bash
   make build && ./bin/server
   ```

2. Open `ios_player/SGAIPlayer/SGAIPlayer.xcodeproj` in Xcode.
3. Select an iOS Simulator (or a physical iPhone on the same Wi-Fi network) and run (**Cmd+R**).
   - On Simulator, leave the server URL as `http://localhost:8080`.
   - On a physical iPhone, change the server URL to `http://<your-mac-ip>:8080`.
