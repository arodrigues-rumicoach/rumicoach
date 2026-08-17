import { useState, useRef, useCallback } from 'react'
import { isWeb } from '../adapters/platform'
import { getVoiceSample } from '../utils/voices'
import { useI18n } from '../i18n'
import { useAudio } from './useAudio'

export function useVoicePreview() {
  const { locale } = useI18n()
  const { fadeOut, fadeIn } = useAudio()
  const [playingVoice, setPlayingVoice] = useState<string | null>(null)
  const [loadingVoice, setLoadingVoice] = useState<string | null>(null)
  const audioRef = useRef<any>(null)

  const stop = useCallback(() => {
    if (audioRef.current) {
      try { audioRef.current.pause() } catch { }
      if (!isWeb) {
        try { audioRef.current.remove?.() } catch { }
      }
      audioRef.current = null
    }
    setPlayingVoice(null)
    setLoadingVoice(null)
    fadeIn(300)
  }, [fadeIn])

  const play = useCallback(async (voiceId: string) => {
    if (playingVoice === voiceId) {
      stop()
      return
    }

    stop()
    fadeOut(200)
    setLoadingVoice(voiceId)

    const sampleUrl = await getVoiceSample(voiceId, locale)
    if (__DEV__) console.log('[VOICE_PREVIEW] sampleUrl:', sampleUrl)
    if (!sampleUrl) {
      setLoadingVoice(null)
      fadeIn(200)
      return
    }

    try {
      if (isWeb) {
        const audio = new Audio(sampleUrl)
        audio.volume = 1
        audioRef.current = audio
        audio.onended = () => { setPlayingVoice(null); audioRef.current = null; fadeIn(300) }
        audio.onerror = () => { setPlayingVoice(null); setLoadingVoice(null); audioRef.current = null; fadeIn(300) }
        await audio.play()
        setLoadingVoice(null)
        setPlayingVoice(voiceId)
      } else {
        const { createAudioPlayer, setAudioModeAsync } = await import('expo-audio')
        await setAudioModeAsync({ playsInSilentMode: true })
        const player = createAudioPlayer({ uri: sampleUrl })
        audioRef.current = player
        const sub = player.addListener('playbackStatusUpdate', (status: any) => {
          if (status.didJustFinish) {
            setPlayingVoice(null)
            audioRef.current = null
            sub.remove()
            fadeIn(300)
          }
        })
        player.play()
        setLoadingVoice(null)
        setPlayingVoice(voiceId)
      }
    } catch (e) {
      if (__DEV__) console.error('[VOICE_PREVIEW] play failed:', e)
      setPlayingVoice(null)
      setLoadingVoice(null)
      audioRef.current = null
      fadeIn(200)
    }
  }, [playingVoice, stop, locale, fadeOut, fadeIn])

  return { playingVoice, loadingVoice, play, stop }
}
