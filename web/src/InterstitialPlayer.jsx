import { useEffect, useRef, useState } from 'react'
import Hls from 'hls.js'

function videoHasAudio(video, hlsInstance) {
  if (hlsInstance?.audioTracks?.length > 0) return true
  if (typeof video.mozHasAudio === 'boolean') return video.mozHasAudio
  if (typeof video.webkitAudioDecodedByteCount === 'number' && video.webkitAudioDecodedByteCount > 0) {
    return true
  }
  if (video.audioTracks && video.audioTracks.length > 0) return true
  return false
}

export default function InterstitialPlayer({
  streamUrl,
  isLive = false,
  autoPlay = false,
  emptyMessage = 'No stream loaded.',
}) {
  const primaryVideoRef = useRef(null)
  const adVideoRef = useRef(null)
  const adHlsRef = useRef(null)
  const breaksRef = useRef([])
  const triggeredIdsRef = useRef(new Set())
  const adModeRef = useRef(null) // null | 'loading' | 'fullscreen' | 'squeezeback'
  const wasMutedRef = useRef(false)
  const mutedByAdRef = useRef(false)

  const [adMode, setAdMode] = useState(null)
  const [adCountdown, setAdCountdown] = useState(null)
  const [adButton, setAdButton] = useState(null)
  const [adButtonClicked, setAdButtonClicked] = useState(false)
  const [playerError, setPlayerError] = useState(null)

  useEffect(() => {
    const primaryVideo = primaryVideoRef.current
    const adVideo = adVideoRef.current
    if (!primaryVideo || !adVideo) return

    breaksRef.current = []
    triggeredIdsRef.current = new Set()
    adModeRef.current = null
    mutedByAdRef.current = false
    setAdMode(null)
    setAdCountdown(null)
    setAdButton(null)
    setAdButtonClicked(false)
    setPlayerError(null)

    if (!streamUrl) {
      primaryVideo.removeAttribute('src')
      primaryVideo.load()
      return
    }

    let destroyed = false

    const mutePrimaryForAd = () => {
      if (mutedByAdRef.current) return
      wasMutedRef.current = primaryVideo.muted
      primaryVideo.muted = true
      mutedByAdRef.current = true
    }

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
      if (!destroyed) {
        if (mutedByAdRef.current) {
          primaryVideo.muted = wasMutedRef.current
          mutedByAdRef.current = false
        }
        if (wasFullscreen && !isLive) {
          primaryVideo.play().catch(() => {})
        }
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
          mutePrimaryForAd()
          if (!isLive) {
            primaryVideo.pause()
          }
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

    const onAdAudioCheck = () => {
      if (!adModeRef.current || mutedByAdRef.current) return
      if (videoHasAudio(adVideo, adHlsRef.current)) {
        mutePrimaryForAd()
      }
    }
    const onAdEnded = () => finishAd()
    const onAdError = () => finishAd()
    adVideo.addEventListener('loadeddata', onAdAudioCheck)
    adVideo.addEventListener('playing', onAdAudioCheck)
    adVideo.addEventListener('timeupdate', onAdAudioCheck)
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
      primaryHls.loadSource(streamUrl)
      primaryHls.attachMedia(primaryVideo)

      if (autoPlay) {
        primaryHls.on(Hls.Events.MANIFEST_PARSED, () => {
          primaryVideo.play().catch(() => {})
        })
      }

      primaryHls.on(Hls.Events.LEVEL_LOADED, (_event, data) => {
        const details = data?.details
        const frag0 = details?.fragments?.[0]
        const basePdt = frag0?.programDateTime
        if (!details?.dateRanges || !basePdt) return

        // Map DATERANGE wall-clock START-DATE to video.currentTime using frag0.start
        // so both VOD and sliding-window live playlists stay aligned.
        const baseTime = frag0.start || 0
        const parsed = Object.values(details.dateRanges)
          .filter((dr) => dr && dr.class === 'com.apple.hls.interstitial')
          .map((dr) => ({
            id: dr.id,
            time: baseTime + (dr.startDate.getTime() - basePdt) / 1000,
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
      primaryVideo.src = streamUrl
      if (autoPlay) {
        primaryVideo.play().catch(() => {})
      }
    } else {
      setPlayerError('HLS is not supported in this browser.')
    }

    return () => {
      destroyed = true
      primaryVideo.removeEventListener('timeupdate', onTimeUpdate)
      adVideo.removeEventListener('loadeddata', onAdAudioCheck)
      adVideo.removeEventListener('playing', onAdAudioCheck)
      adVideo.removeEventListener('timeupdate', onAdAudioCheck)
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
  }, [streamUrl, isLive, autoPlay])

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
    <div className="player-wrapper">
      <video
        ref={primaryVideoRef}
        className={`hls-player primary-player ${adMode === 'squeezeback' ? 'squeezed' : ''}`}
        controls={Boolean(streamUrl) && adMode !== 'fullscreen'}
        playsInline
      />
      <video
        ref={adVideoRef}
        className={`hls-player ad-player ${adMode ? `active ${adMode}` : ''}`}
        playsInline
      />
      {!streamUrl && <div className="player-placeholder">{emptyMessage}</div>}
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
  )
}
