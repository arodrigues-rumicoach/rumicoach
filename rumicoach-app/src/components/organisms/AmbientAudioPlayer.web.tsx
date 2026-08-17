import { useRef, useEffect, useCallback } from 'react'
import { useSettings } from '@/hooks/useSettings'
import { useAudio } from '@/hooks/useAudio'
import { useSession } from '@/hooks/useSession'
import { getThemeAssetUrl } from '@/utils/assetManager'

function attachInteractionListeners(onInteraction: () => void) {
  const handler = () => {
    onInteraction()
    document.removeEventListener('click', handler, true)
    document.removeEventListener('keydown', handler, true)
    document.removeEventListener('touchstart', handler, true)
    document.removeEventListener('pointerdown', handler, true)
  }
  document.addEventListener('click', handler, { capture: true, once: true })
  document.addEventListener('keydown', handler, { capture: true, once: true })
  document.addEventListener('touchstart', handler, { capture: true, once: true })
  document.addEventListener('pointerdown', handler, { capture: true, once: true })
}

export function AmbientAudioPlayer() {
  const { theme } = useSettings()
  const { isMusicEnabled, volume } = useAudio()
  const { status } = useSession()

  const activeAudioRef = useRef<HTMLAudioElement | null>(null)
  const playingAudiosRef = useRef<Set<HTMLAudioElement>>(new Set())
  const fadeIntervalsRef = useRef<Set<ReturnType<typeof setInterval>>>(new Set())
  const isFadingRef = useRef(false)
  const isMusicEnabledRef = useRef(isMusicEnabled)
  const volumeRef = useRef(volume)
  const audioUrlRef = useRef('')
  const isMountedRef = useRef(true)
  const playNextRef = useRef<(url: string, isLoopTransition?: boolean) => void>(() => {})
  const prevStatusRef = useRef(status)
  const prevThemeRef = useRef(theme)

  useEffect(() => {
    isMusicEnabledRef.current = isMusicEnabled
    volumeRef.current = volume
    if (!isFadingRef.current && activeAudioRef.current) {
      activeAudioRef.current.volume = isMusicEnabled ? volume : 0
    }
  }, [isMusicEnabled, volume])

  const clearAllFadeIntervals = useCallback(() => {
    fadeIntervalsRef.current.forEach(clearInterval)
    fadeIntervalsRef.current.clear()
  }, [])

  const stopAll = useCallback(() => {
    clearAllFadeIntervals()
    playingAudiosRef.current.forEach(a => a.pause())
    playingAudiosRef.current.clear()
    activeAudioRef.current = null
    isFadingRef.current = false
  }, [clearAllFadeIntervals])

  const playNext = useCallback((url: string, isLoopTransition = false) => {
    if (!isMountedRef.current || !isMusicEnabledRef.current || !url) return

    if (!isLoopTransition && activeAudioRef.current && !activeAudioRef.current.paused) {
      return
    }

    const audio = new Audio()
    audio.preload = 'auto'
    audio.src = url

    const targetVol = isMusicEnabledRef.current ? volumeRef.current : 0
    audio.volume = isLoopTransition ? 0 : targetVol

    activeAudioRef.current = audio
    playingAudiosRef.current.add(audio)

    const cleanup = () => {
      playingAudiosRef.current.delete(audio)
    }
    audio.addEventListener('ended', cleanup)
    audio.addEventListener('pause', cleanup)

    let nextTriggered = false
    audio.addEventListener('timeupdate', () => {
      const duration = audio.duration
      if (isNaN(duration) || duration <= 0) return

      if (!nextTriggered && audio.currentTime >= duration - 1.0) {
        nextTriggered = true
        playNextRef.current(url, true)

        const fadeStart = Date.now()
        const fadeDuration = 1000
        isFadingRef.current = true

        const fadeInterval = setInterval(() => {
          if (!isMountedRef.current) {
            clearInterval(fadeInterval)
            fadeIntervalsRef.current.delete(fadeInterval)
            return
          }

          const elapsed = Date.now() - fadeStart
          const progress = Math.min(elapsed / fadeDuration, 1)
          const currentTargetVol = isMusicEnabledRef.current ? volumeRef.current : 0

          const fadeOutFactor = Math.cos(progress * Math.PI / 2)
          const fadeInFactor = Math.sin(progress * Math.PI / 2)

          audio.volume = fadeOutFactor * currentTargetVol

          const nextAudio = activeAudioRef.current
          if (nextAudio && nextAudio !== audio) {
            nextAudio.volume = fadeInFactor * currentTargetVol
          }

          if (progress >= 1) {
            clearInterval(fadeInterval)
            fadeIntervalsRef.current.delete(fadeInterval)
            audio.pause()
            audio.src = ''
            cleanup()
            isFadingRef.current = false
          }
        }, 30)

        fadeIntervalsRef.current.add(fadeInterval)
      }
    })

    audio.play().catch(() => {
      if (isMountedRef.current) {
        attachInteractionListeners(() => {
          if (isMountedRef.current && activeAudioRef.current) {
            activeAudioRef.current.play().catch(() => {})
          }
        })
      }
    })
  }, [])

  useEffect(() => {
    playNextRef.current = playNext
  }, [playNext])

  // Mount / theme change
  useEffect(() => {
    isMountedRef.current = true

    const loadAndPlay = async () => {
      const url = getThemeAssetUrl(theme, 'audio')
      if (!isMountedRef.current || !url) return

      audioUrlRef.current = url
      stopAll()

      if (isMusicEnabled) {
        playNext(url)
      }
    }

    loadAndPlay()

    return () => {
      isMountedRef.current = false
      stopAll()
    }
  }, [theme]) // eslint-disable-line react-hooks/exhaustive-deps

  // Volume changes
  useEffect(() => {
    if (isFadingRef.current) return
    const audio = activeAudioRef.current
    if (audio) {
      audio.volume = isMusicEnabled ? volume : 0
    }
  }, [volume, isMusicEnabled])

  // Music toggle
  useEffect(() => {
    if (isMusicEnabled) {
      if (audioUrlRef.current && (!activeAudioRef.current || activeAudioRef.current.paused)) {
        playNext(audioUrlRef.current)
      }
    } else {
      stopAll()
    }
  }, [isMusicEnabled, playNext, stopAll])

  // Session fade
  useEffect(() => {
    const audio = activeAudioRef.current
    if (!audio || !isMusicEnabled) {
      prevStatusRef.current = status
      return
    }

    const wasDisconnected = prevStatusRef.current === 'disconnected'
    const isNowConnected = status === 'connecting' || status === 'connected'
    if (wasDisconnected && isNowConnected && audioUrlRef.current) {
      playNext(audioUrlRef.current)
    }

    const wasConnected = prevStatusRef.current === 'connecting' || prevStatusRef.current === 'connected'
    const isNowDisconnected = status === 'disconnected'
    if (wasConnected && isNowDisconnected) {
      const fadeOutStart = Date.now()
      const fadeOutDuration = 2000
      const startVol = audio.volume
      const fadeOutInterval = setInterval(() => {
        const elapsed = Date.now() - fadeOutStart
        const progress = Math.min(elapsed / fadeOutDuration, 1)
        audio.volume = startVol * (1 - progress)
        if (progress >= 1) {
          clearInterval(fadeOutInterval)
          audio.pause()
        }
      }, 30)
      fadeIntervalsRef.current.add(fadeOutInterval)
    }

    prevStatusRef.current = status
  }, [status, isMusicEnabled, playNext])

  return null
}
