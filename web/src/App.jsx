import { useEffect, useRef, useState } from 'react'
import Hls from 'hls.js'

const STREAM_URL = '/priv/big_buck_bunny/x36xhzz.m3u8'

export default function App() {
  const primaryVideoRef = useRef(null)
  const adVideoRef = useRef(null)
  const adHlsRef = useRef(null)
  const breaksRef = useRef([])
  const triggeredIdsRef = useRef(new Set())
  const adModeRef = useRef(null) // null | 'fullscreen' | 'squeezeback'

  const [adMode, setAdMode] = useState(null)
  const [adCountdown, setAdCountdown] = useState(null)
  const [adButton, setAdButton] = useState(null)
  const [adButtonClicked, setAdButtonClicked] = useState(false)
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
    const primaryVideo = primaryVideoRef.current
    const adVideo = adVideoRef.current
    if (!primaryVideo || !adVideo) return

    let destroyed = false

    const finishAd = () => {
      const wasFullscreen = adModeRef.current === 'fullscreen'
      if (adHlsRef.current) {
        adHlsRef.current.destroy()
        adHlsRef.current = null
      }
      adVideo.removeAttribute('src')
      adVideo.load()
      adModeRef.current = null
      setAdMode(null)
      setAdButton(null)
      setAdButtonClicked(false)
      if (!destroyed && wasFullscreen) {
        primaryVideo.play().catch(() => {})
      }
    }

    const startAd = async (item) => {
      adModeRef.current = 'loading'
      setAdCountdown(null)
      setAdButton(null)
      setAdButtonClicked(false)

      try {
        let adUri = item.assetUri
        let layout = ''
        let buttonMeta = null
        if (!adUri && item.assetListUrl) {
          const assetListUrl = new URL(item.assetListUrl, window.location.origin)
          assetListUrl.searchParams.set('clickable', '1')
          assetListUrl.searchParams.set('squeezeback', '1')
          const res = await fetch(assetListUrl.pathname + assetListUrl.search)
          if (!res.ok) throw new Error(`Ad query HTTP ${res.status}`)
          const data = await res.json()
          const asset = data?.ASSETS?.[0]
          adUri = asset?.URI
          layout = asset?.LAYOUT || ''
          buttonMeta = asset?.BUTTON || null
        }
        if (!adUri || destroyed) {
          finishAd()
          return
        }

        const mode = layout === 'l-squeezeback' ? 'squeezeback' : 'fullscreen'
        adModeRef.current = mode
        setAdMode(mode)
        setAdButton(buttonMeta)
        if (mode === 'fullscreen') {
          primaryVideo.pause()
        }

        if (adUri.endsWith('.m3u8') && Hls.isSupported()) {
          const adHls = new Hls()
          adHlsRef.current = adHls
          adHls.loadSource(adUri)
          adHls.attachMedia(adVideo)
          adHls.on(Hls.Events.MANIFEST_PARSED, () => {
            adVideo.play().catch(() => finishAd())
          })
          adHls.on(Hls.Events.ERROR, (_ev, data) => {
            if (data.fatal) finishAd()
          })
        } else {
          adVideo.src = adUri
          adVideo.play().catch(() => finishAd())
        }
      } catch (err) {
        setPlayerError(`Ad error: ${err.message}`)
        finishAd()
      }
    }

    const onAdEnded = () => finishAd()
    const onAdError = () => finishAd()
    adVideo.addEventListener('ended', onAdEnded)
    adVideo.addEventListener('error', onAdError)

    const onTimeUpdate = () => {
      if (adModeRef.current) return
      const now = primaryVideo.currentTime
      const nextBreak = breaksRef.current.find(
        (b) => !triggeredIdsRef.current.has(b.id) && now >= b.time && now < b.time + 2
      )
      if (nextBreak) {
        triggeredIdsRef.current.add(nextBreak.id)
        startAd(nextBreak)
        return
      }

      const upcoming = breaksRef.current.find(
        (b) => !triggeredIdsRef.current.has(b.id) && b.time - now > 0 && b.time - now <= 5
      )
      setAdCountdown(upcoming ? Math.ceil(upcoming.time - now) : null)
    }
    primaryVideo.addEventListener('timeupdate', onTimeUpdate)

    let primaryHls = null
    if (Hls.isSupported()) {
      primaryHls = new Hls({ enableInterstitialPlayback: false })
      primaryHls.loadSource(STREAM_URL)
      primaryHls.attachMedia(primaryVideo)

      primaryHls.on(Hls.Events.LEVEL_LOADED, (_event, data) => {
        const details = data?.details
        const basePdt = details?.fragments?.[0]?.programDateTime
        if (!details?.dateRanges || !basePdt) return

        const parsed = Object.values(details.dateRanges)
          .filter((dr) => dr && dr.class === 'com.apple.hls.interstitial')
          .map((dr) => ({
            id: dr.id,
            time: (dr.startDate.getTime() - basePdt) / 1000,
            assetListUrl: dr.attr?.['X-ASSET-LIST'] || '',
            assetUri: dr.attr?.['X-ASSET-URI'] || '',
          }))
          .sort((a, b) => a.time - b.time)

        breaksRef.current = parsed
      })

      primaryHls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          setPlayerError(`HLS error: ${data.details}`)
        }
      })
    } else if (primaryVideo.canPlayType('application/vnd.apple.mpegurl')) {
      primaryVideo.src = STREAM_URL
    } else {
      setPlayerError('HLS is not supported in this browser.')
    }

    return () => {
      destroyed = true
      primaryVideo.removeEventListener('timeupdate', onTimeUpdate)
      adVideo.removeEventListener('ended', onAdEnded)
      adVideo.removeEventListener('error', onAdError)
      if (adHlsRef.current) {
        adHlsRef.current.destroy()
        adHlsRef.current = null
      }
      if (primaryHls) {
        primaryHls.destroy()
      }
    }
  }, [])

  const handleAdButtonClick = () => {
    if (!adButton?.URL) return
    if (adButton.ACTION === 'request') {
      fetch(adButton.URL, { method: 'POST' })
        .then(() => setAdButtonClicked(true))
        .catch(() => {})
      return
    }
    window.open(adButton.URL, '_blank', 'noopener,noreferrer')
  }

  return (
    <main className="container">
      <h1>SGAI</h1>
      <p>Go + Chi web service with an embedded React JS application.</p>
      <div className="player-wrapper">
        <video
          ref={primaryVideoRef}
          className={`hls-player primary-player ${adMode === 'squeezeback' ? 'squeezed' : ''}`}
          controls={adMode !== 'fullscreen'}
          playsInline
        />
        <video
          ref={adVideoRef}
          className={`hls-player ad-player ${adMode ? `active ${adMode}` : ''}`}
          playsInline
        />
        {adMode && <span className="ad-badge">Ad</span>}
        {adMode && adButton && (
          <button
            type="button"
            className="ad-cta-button"
            onClick={handleAdButtonClick}
          >
            {adButtonClicked ? 'Clicked!' : adButton.LABEL}
          </button>
        )}
        {!adMode && adCountdown !== null && (
          <span className="ad-badge">Ad in {adCountdown}s</span>
        )}
        {playerError && <p className="player-error">{playerError}</p>}
      </div>
      <div className={`status ${health.state}`}>{health.text}</div>
    </main>
  )
}
