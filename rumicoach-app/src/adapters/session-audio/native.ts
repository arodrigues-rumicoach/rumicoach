import { initialize, playPCMData, stopPlayback, tearDown, toggleRecording } from '@speechmatics/expo-two-way-audio'
import type { SessionAudioAdapter } from './index'

class NativeSessionAudio implements SessionAudioAdapter {
  private muted = false
  private initialized = false

  async initialize(): Promise<void> {
    let ok = await initialize()
    if (!ok) {
      // The engine can fail to start when the audio session was left in a bad
      // state by another audio subsystem. Reset and retry once before giving up.
      if (__DEV__) console.error('[SessionAudio] native initialize failed, retrying after teardown')
      tearDown()
      ok = await initialize()
    }
    this.initialized = !!ok
    if (!this.initialized && __DEV__) {
      console.error('[SessionAudio] native engine failed to initialize')
    }
  }

  teardown(): void {
    toggleRecording(false)
    tearDown()
    this.initialized = false
  }

  startRecording(): void {
    if (!this.muted && this.initialized) toggleRecording(true)
  }

  stopRecording(): void {
    toggleRecording(false)
  }

  playPCMData(pcmData: Uint8Array): void {
    playPCMData(pcmData)
  }

  stopPlayback(): void {
    stopPlayback()
  }

  setMuted(muted: boolean): void {
    this.muted = muted
    if (this.initialized) {
      toggleRecording(!muted)
    }
  }
}

const nativeSessionAudio = new NativeSessionAudio()
export default nativeSessionAudio
