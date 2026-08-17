import { isWeb } from '../platform'

export interface SessionAudioAdapterOptions {
  onMicrophoneData: (data: ArrayBuffer) => void
  onInputVolumeLevel: (level: number) => void
  onOutputVolumeLevel: (level: number) => void
}

export interface SessionAudioAdapter {
  initialize(options?: SessionAudioAdapterOptions): Promise<void>
  teardown(): void
  startRecording(): void
  stopRecording(): void
  playPCMData(pcmData: Uint8Array): void
  stopPlayback(): void
  setMuted(muted: boolean): void
}

export const getSessionAudioAdapter = (): SessionAudioAdapter => {
  if (isWeb) {
    return require('./web').default as SessionAudioAdapter
  }
  return require('./native').default as SessionAudioAdapter
}
