import { useRef, useCallback } from 'react'
import { Platform } from 'react-native'
import { setAudioModeAsync } from 'expo-audio'
import { isWeb } from '@/adapters/platform'
import { getSessionAudioAdapter } from '@/adapters/session-audio'
import type { SessionAudioAdapterOptions } from '@/adapters/session-audio'
import type { MicrophoneDataEvent, VolumeLevelEvent } from '@speechmatics/expo-two-way-audio'

declare const require: (id: string) => any

const resample24To16 = (input: Int16Array): Int16Array => {
  const inputLen = input.length
  const outputLen = Math.floor(inputLen * (16000 / 24000))
  const output = new Int16Array(outputLen)
  const ratio = 24000 / 16000
  for (let i = 0; i < outputLen; i++) {
    const inputIdx = i * ratio
    const idxFloor = Math.floor(inputIdx)
    const idxCeil = Math.min(idxFloor + 1, inputLen - 1)
    const fraction = inputIdx - idxFloor
    output[i] = input[idxFloor] + (input[idxCeil] - input[idxFloor]) * fraction
  }
  return output
}

export const configureAudioMode = async () => {
  try {
    await setAudioModeAsync({
      allowsRecording: true,
      playsInSilentMode: true,
      interruptionMode: 'doNotMix',
      shouldPlayInBackground: false,
      shouldRouteThroughEarpiece: false,
    })
  } catch (error) {
    if (__DEV__) console.error('Failed to set audio mode', error)
  }
}

export interface UseWebSocketOptions {
  wsRef: React.MutableRefObject<WebSocket | null>
  onMessage: (data: string | ArrayBuffer) => void
  onOpen: () => void
  // The close event carries the code the server hung up with, which is how a refused
  // session says why — see WS_CLOSE_INSUFFICIENT_BALANCE in SessionContext.
  onClose: (event?: { code?: number }) => void
  onError: (event: Event) => void
  setInputVolume: (volume: number) => void
  setVolume: (volume: number) => void
  statusRef: React.MutableRefObject<string>
}

export function useWebSocket({ wsRef, onMessage, onOpen, onClose, onError, setInputVolume, setVolume, statusRef }: UseWebSocketOptions) {
  const silenceStartRef = useRef(0)

  const audioOptions: SessionAudioAdapterOptions = {
    onMicrophoneData: (data) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(data)
      }
    },
    onInputVolumeLevel: (level) => setInputVolume(level),
    onOutputVolumeLevel: (level) => setVolume(level),
  }

  const setupNativeListeners = useCallback(() => {
    if (isWeb) return

    const onMicrophoneData = (event: MicrophoneDataEvent) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(event.data)
      }
    }

    const onInputVolumeLevel = (event: VolumeLevelEvent) => {
      setInputVolume(event.data)
      if (wsRef.current?.readyState !== WebSocket.OPEN) return
      if (statusRef.current === 'speaking') {
        if (event.data > 0.08) {
          statusRef.current = 'listening'
        } else {
          return
        }
      }

      if (event.data < 0.05) {
        if (silenceStartRef.current === 0) {
          silenceStartRef.current = Date.now()
        } else if (Date.now() - silenceStartRef.current > 800) {
          //wsRef.current?.send(JSON.stringify({ type: 'turn_complete' }))
          silenceStartRef.current = 0
        }
      } else {
        silenceStartRef.current = 0
      }
    }

    const onOutputVolumeLevel = (event: VolumeLevelEvent) => {
      setVolume(event.data)
    }

    let cleanedUp = false
    let removeListeners: (() => void) | null = null

    const setup = () => {
      const { addExpoTwoWayAudioEventListener } = require('@speechmatics/expo-two-way-audio')
      if (cleanedUp) return
      const subs = [
        addExpoTwoWayAudioEventListener('onMicrophoneData', onMicrophoneData),
        addExpoTwoWayAudioEventListener('onInputVolumeLevelData', onInputVolumeLevel),
        addExpoTwoWayAudioEventListener('onOutputVolumeLevelData', onOutputVolumeLevel),
      ]
      removeListeners = () => subs.forEach((sub) => sub.remove())
    }

    setup()

    return () => {
      cleanedUp = true
      removeListeners?.()
    }
  }, [setInputVolume, setVolume, statusRef, wsRef])

  const handleAudioBinary = useCallback((data: ArrayBuffer) => {
    const pcm24k = new Int16Array(data)
    const pcm16k = resample24To16(pcm24k)
    getSessionAudioAdapter().playPCMData(new Uint8Array(pcm16k.buffer))
    return { pcm16k }
  }, [])

  const cleanupWs = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.onclose = null
      wsRef.current.onmessage = null
      wsRef.current.onerror = null
      wsRef.current.close()
      wsRef.current = null
    }
  }, [wsRef])

  return {
    audioOptions,
    setupNativeListeners,
    handleAudioBinary,
    cleanupWs,
    configureAudioMode,
  }
}
