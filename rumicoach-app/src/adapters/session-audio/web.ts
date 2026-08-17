import type { SessionAudioAdapter, SessionAudioAdapterOptions } from './index'

const floatTo16BitPCM = (input: Float32Array): Int16Array => {
  const output = new Int16Array(input.length)
  for (let i = 0; i < input.length; i++) {
    const s = Math.max(-1, Math.min(1, input[i]))
    output[i] = s < 0 ? s * 0x8000 : s * 0x7fff
  }
  return output
}

class WebSessionAudio implements SessionAudioAdapter {
  private ctx: AudioContext | null = null
  private stream: MediaStream | null = null
  private processor: ScriptProcessorNode | null = null
  private nextStartTime = 0
  private sources: AudioBufferSourceNode[] = []
  private vad: unknown = null
  private options: SessionAudioAdapterOptions | null = null
  private muted = false

  async initialize(options?: SessionAudioAdapterOptions): Promise<void> {
    this.options = options || null
    const AudioContextCtor =
      window.AudioContext ||
      ((window as unknown as Record<string, unknown>).webkitAudioContext as typeof AudioContext)
    this.ctx = new AudioContextCtor()
    this.nextStartTime = this.ctx.currentTime
  }

  async startRecording(): Promise<void> {
    if (!this.ctx) return
    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      })
    } catch (e) {
      console.error('Mic permission error', e)
      return
    }

    // Acquiring the microphone is slow — a permission prompt the first time — and
    // teardown() can land in that gap, which nulls the context. A session refused for
    // balance is exactly this shape: the socket opens (so recording starts), the server
    // sends its refusal and closes, and the teardown arrives while this await is still
    // pending. Checking only on the way in left createMediaStreamSource dereferencing a
    // null context.
    //
    // Release the stream rather than just returning: the microphone is live by now, and
    // there is no longer a session to feed it to.
    if (!this.ctx) {
      stream.getTracks().forEach((t) => t.stop())
      return
    }

    this.stream = stream
    const source = this.ctx.createMediaStreamSource(stream)
    const processor = this.ctx.createScriptProcessor(4096, 1, 1)
    this.processor = processor

    source.connect(processor)
    const muteGain = this.ctx.createGain()
    muteGain.gain.value = 0
    processor.connect(muteGain)
    muteGain.connect(this.ctx.destination)

    processor.onaudioprocess = (e) => {
      if (this.muted) return
      const input = e.inputBuffer.getChannelData(0)
      const inRate = this.ctx!.sampleRate
      const outRate = 16000
      const ratio = inRate / outRate
      const newLength = Math.floor(input.length / ratio)
      const resampled = new Float32Array(newLength)
      for (let i = 0; i < newLength; i++) {
        resampled[i] = input[Math.floor(i * ratio)]
      }

      let sum = 0
      for (let i = 0; i < resampled.length; i++) sum += resampled[i] * resampled[i]
      const volume = Math.sqrt(sum / resampled.length) * 5
      this.options?.onInputVolumeLevel(volume)

      const pcm = floatTo16BitPCM(resampled)
      this.options?.onMicrophoneData(pcm.buffer as ArrayBuffer)
    }
  }

  stopRecording(): void {
    if (this.processor) {
      this.processor.disconnect()
      this.processor = null
    }
    if (this.stream) {
      this.stream.getTracks().forEach((t) => t.stop())
      this.stream = null
    }
  }

  playPCMData(pcmData: Uint8Array): void {
    if (!this.ctx || pcmData.length === 0) return
    const int16 = new Int16Array(pcmData.buffer)
    const float32 = new Float32Array(int16.length)
    for (let i = 0; i < int16.length; i++) float32[i] = int16[i] / 32768.0

    const buffer = this.ctx.createBuffer(1, float32.length, 16000)
    buffer.copyToChannel(float32, 0)

    const source = this.ctx.createBufferSource()
    source.buffer = buffer
    source.connect(this.ctx.destination)

    const now = this.ctx.currentTime
    if (this.nextStartTime < now) this.nextStartTime = now
    source.start(this.nextStartTime)
    this.nextStartTime += buffer.duration
    this.sources.push(source)

    let sum = 0
    for (let i = 0; i < float32.length; i++) sum += Math.abs(float32[i])
    const volume = (sum / float32.length) * 5
    this.options?.onOutputVolumeLevel(volume)
  }

  stopPlayback(): void {
    this.sources.forEach((s) => {
      try {
        s.stop()
      } catch {}
    })
    this.sources = []
    this.nextStartTime = this.ctx?.currentTime ?? 0
    this.options?.onOutputVolumeLevel(0)
  }

  teardown(): void {
    this.stopRecording()
    this.sources.forEach((s) => {
      try {
        s.stop()
      } catch {}
    })
    this.sources = []
    if (this.ctx) {
      try {
        this.ctx.close()
      } catch {}
      this.ctx = null
    }
  }

  setMuted(muted: boolean): void {
    this.muted = muted
  }
}

const webSessionAudio = new WebSessionAudio()
export default webSessionAudio
