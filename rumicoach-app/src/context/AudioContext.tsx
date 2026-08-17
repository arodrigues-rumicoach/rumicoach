import { createContext, useState, useCallback, useEffect, useRef, useMemo, type ReactNode } from 'react'
import { AppState, type AppStateStatus } from 'react-native'
import { useSegments } from 'expo-router'
import { getStorageAdapter } from '../adapters/storage'
import { isWeb } from '../adapters/platform'
import { useSettings } from '../hooks/useSettings'
import { useThemeAssetUri } from '../hooks/useThemeAssetUri'

const VOLUME_KEY = 'rumi_music_volume'

type AudioPlayer = {
  volume: number
  playing: boolean
  currentTime: number
  duration: number
  loop: boolean
  play: () => void
  pause: () => void
  replace: (src: number | string) => void
  addListener: (event: string, cb: (status: any) => void) => { remove: () => void }
}

function createNullPlayer(): AudioPlayer {
  return { volume: 0, playing: false, currentTime: 0, duration: 0, loop: false, play() { }, pause() { }, replace() { }, addListener() { return { remove() { } } } }
}

export interface AudioContextType {
  isMusicEnabled: boolean
  toggleMusic: () => void
  setMusicEnabled: (enabled: boolean) => void
  volume: number
  setVolume: (v: number) => void
  fadeOut: (durationMs: number) => void
  fadeIn: (durationMs: number) => void
  pauseAmbient: () => void
  setSplashVisible: (visible: boolean) => void
}

export const AudioContext = createContext<AudioContextType | undefined>(undefined)

export function AudioProvider({ children }: { children: ReactNode }) {
  const { theme } = useSettings()
  const { audioUri, isLoading } = useThemeAssetUri(theme)
  const segments = useSegments()
  const [isMusicEnabled, setIsMusicEnabled] = useState(true)
  const [volume, setVolumeState] = useState<number>(0.3)

  const player1Ref = useRef<AudioPlayer | null>(null)
  const player2Ref = useRef<AudioPlayer | null>(null)
  const isPlayer1ActiveRef = useRef(true)
  const isFadingRef = useRef(false)
  const isSessionFadingRef = useRef(false)
  const nextTriggeredRef = useRef(false)
  const isMusicEnabledRef = useRef(isMusicEnabled)
  const volumeRef = useRef(volume)
  const sessionFadeTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const watchdogTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const hasInteractedRef = useRef(!isWeb)
  const pendingPlayRef = useRef(false)
  const [splashVisible, setSplashVisibleState] = useState(true)
  const splashVisibleRef = useRef(true)
  const isAudioAllowed = (segs: string[]) => segs[0] !== '(auth)' && segs[0] !== 'onboarding'
  const isAllowedRef = useRef(isAudioAllowed(segments))
  const audioUriRef = useRef(audioUri)

  useEffect(() => {
    isMusicEnabledRef.current = isMusicEnabled
    volumeRef.current = volume
  }, [isMusicEnabled, volume])

  useEffect(() => {
    audioUriRef.current = audioUri
  }, [audioUri])

  const playIfAllowed = useCallback((player: AudioPlayer | null | undefined) => {
    if (!player) return
    if (splashVisibleRef.current) return
    if (isWeb && !hasInteractedRef.current) {
      pendingPlayRef.current = true
      return
    }
    try {
      player.play()
    } catch {
      // Ignore autoplay errors (e.g. Safari blocking playback before interaction).
    }
  }, [])

  const getActivePlayer = useCallback(() => isPlayer1ActiveRef.current ? player1Ref.current : player2Ref.current, [])
  const getNextPlayer = useCallback(() => isPlayer1ActiveRef.current ? player2Ref.current : player1Ref.current, [])

  const stopAll = useCallback(() => {
    player1Ref.current?.pause()
    player2Ref.current?.pause()
    nextTriggeredRef.current = false
    isFadingRef.current = false
  }, [])

  const startCrossfade = useCallback(() => {
    if (isFadingRef.current) return

    const currentAudioUri = audioUriRef.current
    if (!currentAudioUri) return

    const activePlayer = getActivePlayer()
    const nextPlayer = getNextPlayer()
    if (!activePlayer || !nextPlayer) return

    isFadingRef.current = true
    if (__DEV__) console.log('[AMBIENT] crossfade starting')

    try {
      nextPlayer.replace(currentAudioUri)
    } catch {
      isFadingRef.current = false
      if (__DEV__) console.log('[AMBIENT] crossfade replace() failed')
      return
    }

    nextPlayer.volume = 0
    playIfAllowed(nextPlayer)

    const startTime = Date.now()
    const fadeDuration = 1000
    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime
      const progress = Math.min(elapsed / fadeDuration, 1)
      const targetVol = isMusicEnabledRef.current ? volumeRef.current : 0

      const fadeOutFactor = Math.cos(progress * Math.PI / 2)
      const fadeInFactor = Math.sin(progress * Math.PI / 2)

      activePlayer.volume = fadeOutFactor * targetVol
      nextPlayer.volume = fadeInFactor * targetVol

      if (progress >= 1) {
        clearInterval(interval)
        activePlayer.pause()
        isPlayer1ActiveRef.current = !isPlayer1ActiveRef.current
        nextTriggeredRef.current = false
        isFadingRef.current = false
        if (__DEV__) console.log('[AMBIENT] crossfade complete')
      }
    }, 30)
  }, [getActivePlayer, getNextPlayer, playIfAllowed])

  const setupListener = useCallback((player: AudioPlayer) => {
    return player.addListener('playbackStatusUpdate', (status: any) => {
      const isActive = isPlayer1ActiveRef.current
        ? player === player1Ref.current
        : player === player2Ref.current
      if (!isActive) return
      if (isFadingRef.current) return

      const duration = status.duration
      if (!duration || duration <= 0) return

      // Start crossfade 2s before track ends — wider window to account for
      // the ~500ms polling interval of playbackStatusUpdate on iOS.
      if (!nextTriggeredRef.current && status.currentTime >= duration - 2.0) {
        nextTriggeredRef.current = true
        startCrossfade()
      }
    })
  }, [startCrossfade])

  const createWebPlayer = useCallback(() => createNullPlayer(), [])

  // Create players on mount
  useEffect(() => {
    const createPlayer = !isWeb
      ? () => { const { createAudioPlayer: cap } = require('expo-audio'); return cap(null); }
      : createWebPlayer
    const p1 = createPlayer()
    const p2 = createPlayer()
    player1Ref.current = p1
    player2Ref.current = p2

    // Native loop ensures audio never goes silent even if crossfade is missed.
    try { p1.loop = true } catch { }
    try { p2.loop = true } catch { }

    if (__DEV__) console.log('[AMBIENT] players created', { isWeb, hasP1: !!p1, hasP2: !!p2 })

    const sub1 = setupListener(p1)
    const sub2 = setupListener(p2)

    return () => {
      sub1.remove()
      sub2.remove()
      p1.pause()
      p2.pause()
    }
  }, [setupListener])

  // Watchdog: periodically check if the active player is still playing.
  // Catches cases where the crossfade trigger was missed and the native loop
  // restarted the track — we detect the position reset and trigger crossfade.
  useEffect(() => {
    if (isWeb) return

    watchdogTimerRef.current = setInterval(() => {
      if (isFadingRef.current || isSessionFadingRef.current) return
      if (!isMusicEnabledRef.current || !isAllowedRef.current) return

      const player = getActivePlayer()
      if (!player || !player.playing) return

      const { currentTime, duration } = player
      if (!duration || duration <= 0) return

      // If we're within 3s of the end and crossfade hasn't started, trigger it.
      if (!nextTriggeredRef.current && currentTime >= duration - 3.0) {
        if (__DEV__) console.log('[AMBIENT] watchdog triggered crossfade', { currentTime, duration })
        nextTriggeredRef.current = true
        startCrossfade()
      }
    }, 2000)

    return () => {
      if (watchdogTimerRef.current) clearInterval(watchdogTimerRef.current)
    }
  }, [getActivePlayer, startCrossfade])

  // Web autoplay guard: defer ambient audio until first user interaction
  useEffect(() => {
    if (!isWeb) return
    const startOnInteraction = () => {
      hasInteractedRef.current = true
      if (!pendingPlayRef.current) return
      pendingPlayRef.current = false
      const player = getActivePlayer()
      if (!player) return
      if (isMusicEnabledRef.current) {
        playIfAllowed(player)
      }
    }
    const win = (globalThis as any).window
    if (!win) return
    win.addEventListener('click', startOnInteraction, { once: true })
    return () => win.removeEventListener('click', startOnInteraction)
  }, [playIfAllowed, getActivePlayer])

  // Load theme and start playing
  useEffect(() => {
    if (!audioUri || isLoading) return

    stopAll()
    isPlayer1ActiveRef.current = true
    nextTriggeredRef.current = false

    const player = player1Ref.current
    if (!player) return

    player.replace(audioUri)
    player.volume = isMusicEnabled ? volume : 0

    if (__DEV__) console.log('[AMBIENT] load theme', { audioUri: audioUri.slice(-30), isMusicEnabled, isAllowed: isAllowedRef.current })
    if (isMusicEnabled && isAllowedRef.current) {
      playIfAllowed(player)
    }
  }, [audioUri, isLoading, playIfAllowed])

  // Handle volume changes (only when not crossfading or session-fading)
  useEffect(() => {
    if (isFadingRef.current) return
    if (isSessionFadingRef.current) return
    const player = getActivePlayer()
    if (player) {
      player.volume = isMusicEnabled ? volume : 0
    }
  }, [volume, isMusicEnabled, getActivePlayer])

  // Handle music toggle
  useEffect(() => {
    const player = getActivePlayer()
    if (!player) return

    if (isMusicEnabled) {
      if (__DEV__) console.log('[AMBIENT] music enabled', { audioUri: !!audioUri, isAllowed: isAllowedRef.current })
      if (audioUri && isAllowedRef.current) {
        playIfAllowed(player)
      }
    } else {
      player.pause()
    }
  }, [isMusicEnabled, audioUri, playIfAllowed, getActivePlayer])

  // Handle app foreground/background transitions
  useEffect(() => {
    const sub = AppState.addEventListener('change', (state: AppStateStatus) => {
      const player = getActivePlayer()
      if (!player) return

      if (state === 'active') {
        if (isMusicEnabledRef.current && !isSessionFadingRef.current && isAllowedRef.current) {
          playIfAllowed(player)
        }
      } else if (state === 'background' || state === 'inactive') {
        player.pause()
      }
    })
    return () => sub.remove()
  }, [playIfAllowed, getActivePlayer])

  // Web: pause ambient audio when the browser tab loses focus
  useEffect(() => {
    if (!isWeb) return

    const handleVisibility = () => {
      const player = getActivePlayer()
      if (!player) return

      if (document.hidden) {
        player.pause()
      } else if (isMusicEnabledRef.current && !isSessionFadingRef.current && isAllowedRef.current) {
        playIfAllowed(player)
      }
    }

    const win = (globalThis as any).window
    if (!win) return
    win.addEventListener('visibilitychange', handleVisibility)
    return () => win.removeEventListener('visibilitychange', handleVisibility)
  }, [playIfAllowed, getActivePlayer])

  // Route awareness: play ambient audio on all screens except (auth) and onboarding
  useEffect(() => {
    const allowed = isAudioAllowed(segments)
    const wasAllowed = isAllowedRef.current
    isAllowedRef.current = allowed

    if (__DEV__) console.log('[AMBIENT] route check', { allowed, wasAllowed, segments: segments.join('/') })

    if (allowed === wasAllowed) return

    const player = getActivePlayer()
    if (!player) return

    if (allowed) {
      // Navigated to an allowed screen: resume audio if music is enabled
      if (isMusicEnabledRef.current && audioUri) {
        player.volume = volumeRef.current
        playIfAllowed(player)
      }
    } else {
      // Navigated to (auth) or onboarding: pause audio
      player.pause()
    }
  }, [segments, audioUri, playIfAllowed, getActivePlayer])

  const toggleMusic = useCallback(() => {
    setIsMusicEnabled(prev => !prev)
  }, [])

  const setMusicEnabled = useCallback((enabled: boolean) => {
    setIsMusicEnabled(enabled)
  }, [])

  const setVolume = useCallback((v: number) => {
    setVolumeState(v)
    getStorageAdapter().setItemAsync(VOLUME_KEY, v.toString()).catch(() => { })
  }, [])

  const setSplashVisible = useCallback((visible: boolean) => {
    splashVisibleRef.current = visible
    setSplashVisibleState(visible)
  }, [])

  const clearSessionFade = useCallback(() => {
    if (sessionFadeTimerRef.current) {
      clearInterval(sessionFadeTimerRef.current)
      sessionFadeTimerRef.current = null
    }
  }, [])

  const pauseAmbient = useCallback(() => {
    clearSessionFade()
    isSessionFadingRef.current = false
    // Zero volume *before* pausing so there is no audible tail while the
    // native player finishes its async teardown.
    if (player1Ref.current) { player1Ref.current.volume = 0; player1Ref.current.pause() }
    if (player2Ref.current) { player2Ref.current.volume = 0; player2Ref.current.pause() }
  }, [clearSessionFade])

  const fadeOut = useCallback((durationMs: number) => {
    clearSessionFade()
    isSessionFadingRef.current = true
    const player = getActivePlayer()
    if (!player) { isSessionFadingRef.current = false; return }

    const startVol = player.volume
    const startTime = Date.now()
    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime
      const progress = Math.min(elapsed / durationMs, 1)
      player.volume = startVol * (1 - progress)
      if (progress >= 1) {
        clearInterval(interval)
        if (sessionFadeTimerRef.current === interval) sessionFadeTimerRef.current = null
        player.pause()
        isSessionFadingRef.current = false
      }
    }, 30)
    sessionFadeTimerRef.current = interval
  }, [clearSessionFade, getActivePlayer])

  const fadeIn = useCallback((durationMs: number) => {
    clearSessionFade()
    isSessionFadingRef.current = true
    const player = getActivePlayer()
    if (!player || !isAllowedRef.current) {
      isSessionFadingRef.current = false
      return
    }

    player.volume = 0
    playIfAllowed(player)

    const targetVol = volumeRef.current
    const startTime = Date.now()
    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime
      const progress = Math.min(elapsed / durationMs, 1)
      player.volume = targetVol * progress
      if (progress >= 1) {
        clearInterval(interval)
        if (sessionFadeTimerRef.current === interval) sessionFadeTimerRef.current = null
        isSessionFadingRef.current = false
      }
    }, 30)
    sessionFadeTimerRef.current = interval
  }, [clearSessionFade, playIfAllowed, getActivePlayer])

  // Pause while the splash screen is visible and resume once it hides.
  useEffect(() => {
    if (splashVisible) {
      stopAll()
      return
    }

    if (!isAllowedRef.current || !isMusicEnabledRef.current || !audioUriRef.current) return

    const player = getActivePlayer()
    if (!player) return

    player.volume = volumeRef.current
    playIfAllowed(player)
  }, [splashVisible, getActivePlayer, playIfAllowed, stopAll])

  const value = useMemo(() => ({
    isMusicEnabled, toggleMusic, setMusicEnabled, volume, setVolume, fadeOut, fadeIn, pauseAmbient, setSplashVisible
  }), [isMusicEnabled, toggleMusic, setMusicEnabled, volume, setVolume, fadeOut, fadeIn, pauseAmbient, setSplashVisible])

  return (
    <AudioContext.Provider value={value}>
      {children}
    </AudioContext.Provider>
  )
}
