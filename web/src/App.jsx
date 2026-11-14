import { useEffect, useRef, useState } from 'react'
import Hls from 'hls.js'

const STREAM_URL = '/priv/big_buck_bunny/x36xhzz.m3u8'

export default function App() {
  const videoRef = useRef(null)
  const [health, setHealth] = useState({ state: 'loading', text: 'Checking API...' })
  const [playerError, setPlayerError] = useState(null)

  useEffect(() => {
    let cancelled = false

    fetch('/api/health')
      .then((res) => {
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`)
        }
        return res.json()
      })
      .then((data) => {
        if (!cancelled) {
          setHealth({ state: 'ok', text: `API status: ${data.status}` })
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setHealth({ state: 'err', text: `API error: ${err.message}` })
        }
      })

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    if (Hls.isSupported()) {
      const hls = new Hls()
      hls.loadSource(STREAM_URL)
      hls.attachMedia(video)
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          setPlayerError(`HLS error: ${data.details}`)
        }
      })
      return () => {
        hls.destroy()
      }
    }

    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = STREAM_URL
      return
    }

    setPlayerError('HLS is not supported in this browser.')
  }, [])

  return (
    <main className="container">
      <h1>sgai</h1>
      <p>Go + Chi web service with an embedded React JS application.</p>
      <div className="player-wrapper">
        <video
          ref={videoRef}
          className="hls-player"
          controls
          playsInline
        />
        {playerError && <p className="player-error">{playerError}</p>}
      </div>
      <div className={`status ${health.state}`}>{health.text}</div>
    </main>
  )
}
