import { describe, expect, it, jest, beforeEach } from '@jest/globals'

// Acquiring the microphone is the one await inside the adapter, and a session refused
// for balance tears down while it is still pending: the socket opens (so recording
// starts), the server sends its refusal and closes, and disconnect() runs. These pin the
// behaviour on both sides of that await.

const makeTrack = () => ({ stop: jest.fn() })

let currentTracks: ReturnType<typeof makeTrack>[]
let resolveMic: (stream: unknown) => void
let rejectMic: (err: unknown) => void

const audioNode = () => ({ connect: jest.fn(), disconnect: jest.fn(), gain: { value: 0 } })

beforeEach(() => {
  jest.resetModules()
  currentTracks = [makeTrack()]

  const ctx = {
    currentTime: 0,
    sampleRate: 48000,
    destination: {},
    createMediaStreamSource: jest.fn(() => audioNode()),
    createScriptProcessor: jest.fn(() => ({ ...audioNode(), onaudioprocess: null })),
    createGain: jest.fn(() => audioNode()),
    close: jest.fn(),
  }

  ;(global as any).window = { AudioContext: jest.fn(() => ctx) }
  ;(global as any).navigator = {
    mediaDevices: {
      getUserMedia: jest.fn(() => new Promise((res, rej) => {
        resolveMic = res as typeof resolveMic
        rejectMic = rej as typeof rejectMic
      })),
    },
  }
})

const loadAdapter = () => require('../session-audio/web').default

describe('WebSessionAudio.startRecording', () => {
  it('wires the stream up when nothing interrupts it', async () => {
    const audio = loadAdapter()
    await audio.initialize()

    const started = audio.startRecording()
    resolveMic({ getTracks: () => currentTracks })
    await started

    // The context survived, so the microphone is actually connected to it.
    expect((global as any).window.AudioContext).toHaveBeenCalled()
    expect(currentTracks[0].stop).not.toHaveBeenCalled()
  })

  // The reported crash: teardown() nulls the context while getUserMedia is still
  // pending, and the resumed startRecording dereferenced it —
  // "Cannot read properties of null (reading 'createMediaStreamSource')".
  it('survives a teardown that lands while the microphone is being acquired', async () => {
    const audio = loadAdapter()
    await audio.initialize()

    const started = audio.startRecording()
    audio.teardown()
    resolveMic({ getTracks: () => currentTracks })

    await expect(started).resolves.toBeUndefined()
  })

  // The microphone is live by the time we find out, and there is no session left to
  // feed it: leaving it open keeps the browser's recording indicator lit.
  it('releases the microphone when it is no longer wanted', async () => {
    const audio = loadAdapter()
    await audio.initialize()

    const started = audio.startRecording()
    audio.teardown()
    resolveMic({ getTracks: () => currentTracks })
    await started

    expect(currentTracks[0].stop).toHaveBeenCalled()
  })

  it('handles a refused microphone permission without throwing', async () => {
    const audio = loadAdapter()
    await audio.initialize()

    const started = audio.startRecording()
    rejectMic(Object.assign(new Error('Permission denied'), { name: 'NotAllowedError' }))

    await expect(started).resolves.toBeUndefined()
  })

  it('does nothing when it was never initialized', async () => {
    const audio = loadAdapter()
    await expect(audio.startRecording()).resolves.toBeUndefined()
    expect((global as any).navigator.mediaDevices.getUserMedia).not.toHaveBeenCalled()
  })
})
