import { useEffect, useState } from 'react'
import InterstitialPlayer from './InterstitialPlayer'

const VOD_STREAM_URL = '/priv/big_buck_bunny/x36xhzz.m3u8'

function modeFromPath(pathname) {
  return pathname === '/live' ? 'live' : 'vod'
}

export default function App() {
  const [mode, setMode] = useState(() => modeFromPath(window.location.pathname))
  const [health, setHealth] = useState({ state: 'loading', text: 'Checking API...' })
  const [liveStatus, setLiveStatus] = useState({ running: false, masterUrl: '' })
  const [liveBusy, setLiveBusy] = useState(false)
  const [liveError, setLiveError] = useState(null)

  useEffect(() => {
    const onPopState = () => {
      setMode(modeFromPath(window.location.pathname))
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  useEffect(() => {
    let cancelled = false

    fetch('/api/health')
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
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
    if (mode !== 'live') return
    let cancelled = false

    fetch('/api/live/status')
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then((data) => {
        if (!cancelled) {
          setLiveStatus(data)
          setLiveError(null)
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setLiveError(`Failed to check live status: ${err.message}`)
        }
      })

    return () => {
      cancelled = true
    }
  }, [mode])

  const switchMode = (nextMode) => {
    if (nextMode === mode) return
    const nextPath = nextMode === 'live' ? '/live' : '/vod'
    window.history.pushState({}, '', nextPath)
    setMode(nextMode)
  }

  const handleStartLive = async () => {
    setLiveBusy(true)
    setLiveError(null)
    try {
      const res = await fetch('/api/live/start', { method: 'POST' })
      const data = await res.json()
      if (!res.ok) {
        throw new Error(data?.error || `HTTP ${res.status}`)
      }
      setLiveStatus(data)
    } catch (err) {
      setLiveError(`Start failed: ${err.message}`)
    } finally {
      setLiveBusy(false)
    }
  }

  const handleStopLive = async () => {
    setLiveBusy(true)
    setLiveError(null)
    try {
      const res = await fetch('/api/live/stop', { method: 'POST' })
      const data = await res.json()
      if (!res.ok) {
        throw new Error(data?.error || `HTTP ${res.status}`)
      }
      setLiveStatus(data)
    } catch (err) {
      setLiveError(`Stop failed: ${err.message}`)
    } finally {
      setLiveBusy(false)
    }
  }

  return (
    <main className="container">
      <nav className="mode-nav" aria-label="Demo mode">
        <button
          type="button"
          className={`mode-tab ${mode === 'vod' ? 'active' : ''}`}
          onClick={() => switchMode('vod')}
        >
          VOD Demo
        </button>
        <button
          type="button"
          className={`mode-tab ${mode === 'live' ? 'active' : ''}`}
          onClick={() => switchMode('live')}
        >
          Linear (Live) Demo
        </button>
      </nav>

      <h1>SGAI</h1>
      <p>
        {mode === 'live'
          ? 'Linear live stream demo looping Big Buck Bunny with on-the-fly HLS interstitials.'
          : 'VOD demo with HLS interstitials (fullscreen and L-squeezeback).'}
      </p>

      {mode === 'live' && (
        <section className="live-controls" aria-label="Live stream controls">
          <div className="live-buttons">
            <button
              type="button"
              className="btn btn-primary"
              onClick={handleStartLive}
              disabled={liveBusy || liveStatus.running}
            >
              {liveBusy && !liveStatus.running ? 'Starting...' : 'Start Live Stream'}
            </button>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleStopLive}
              disabled={liveBusy || !liveStatus.running}
            >
              Stop Live Stream
            </button>
            <span className={`live-pill ${liveStatus.running ? 'running' : 'offline'}`}>
              {liveStatus.running ? 'LIVE' : 'Offline'}
            </span>
          </div>
          {liveStatus.running && liveStatus.masterUrl && (
            <div className="live-manifest-url">
              Manifest: <code>{liveStatus.masterUrl}</code>
            </div>
          )}
          {liveError && <p className="player-error">{liveError}</p>}
        </section>
      )}

      {mode === 'vod' ? (
        <InterstitialPlayer streamUrl={VOD_STREAM_URL} />
      ) : (
        <InterstitialPlayer
          streamUrl={liveStatus.running ? liveStatus.masterUrl : ''}
          isLive
          autoPlay
          emptyMessage='Live stream is offline. Click "Start Live Stream" above to generate and play the endless HLS stream.'
        />
      )}

      <div className={`status ${health.state}`}>{health.text}</div>
    </main>
  )
}
