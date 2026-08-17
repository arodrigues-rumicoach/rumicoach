import { describe, expect, it, jest, beforeEach, afterEach } from '@jest/globals'
import { Platform } from 'react-native'

const mockInitialize = jest.fn<() => Promise<boolean>>()
const mockPlayPCMData = jest.fn<(data: Uint8Array) => void>()
const mockStopPlayback = jest.fn<() => void>()
const mockTearDown = jest.fn<() => void>()
const mockToggleRecording = jest.fn<(recording: boolean) => void>()

jest.mock('@speechmatics/expo-two-way-audio', () => ({
  initialize: mockInitialize,
  playPCMData: mockPlayPCMData,
  stopPlayback: mockStopPlayback,
  tearDown: mockTearDown,
  toggleRecording: mockToggleRecording,
}))

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

const loadAdapterForOS = (os: 'web' | 'ios' | 'android') => {
  jest.resetModules()
  setPlatformOS(os)

  const { getSessionAudioAdapter } = require('../session-audio') as typeof import('../session-audio')
  const { default: expected } = require(
    os === 'web' ? '../session-audio/web' : '../session-audio/native'
  ) as typeof import('../session-audio/web')

  return { adapter: getSessionAudioAdapter(), expected }
}

const setupWebAudioMocks = () => {
  const contexts: MockAudioContext[] = []

  class MockAudioBuffer {
    duration = 0.1
    copyToChannel = jest.fn()
  }

  class MockAudioBufferSourceNode {
    buffer: unknown = null
    connect = jest.fn(() => this)
    start = jest.fn()
    stop = jest.fn()
  }

  class MockAudioContext {
    sampleRate = 48000
    currentTime = 0
    destination = {}
    close = jest.fn()
    createMediaStreamSource = jest.fn(() => ({ connect: jest.fn((node: unknown) => node) }))
    createScriptProcessor = jest.fn(() => ({
      connect: jest.fn((node: unknown) => node),
      disconnect: jest.fn(),
      onaudioprocess: null as ((e: { inputBuffer: { getChannelData: (channel: number) => Float32Array } }) => void) | null,
    }))
    createGain = jest.fn(() => ({ gain: { value: 0 }, connect: jest.fn((node: unknown) => node) }))
    createBuffer = jest.fn(() => new MockAudioBuffer())
    createBufferSource = jest.fn(() => new MockAudioBufferSourceNode())

    constructor() {
      contexts.push(this)
    }
  }

  const trackStop = jest.fn<() => void>()
  const stream = { getTracks: () => [{ stop: trackStop }] } as unknown as MediaStream
  const getUserMedia = jest.fn<() => Promise<MediaStream>>().mockResolvedValue(stream)

  ;(global as any).window = {
    AudioContext: MockAudioContext,
    webkitAudioContext: MockAudioContext,
  }
  ;(global as any).AudioContext = MockAudioContext
  ;(global as any).navigator = { mediaDevices: { getUserMedia } }

  return {
    contexts,
    getUserMedia,
    trackStop,
    stream,
    MockAudioContext,
    MockAudioBufferSourceNode,
    MockAudioBuffer,
  }
}

describe('getSessionAudioAdapter platform selector', () => {
  beforeEach(() => {
    jest.resetModules()
    jest.clearAllMocks()
  })

  it('returns the web adapter when Platform.OS is web', () => {
    const { adapter, expected } = loadAdapterForOS('web')
    expect(adapter).toBe(expected)
  })

  it('returns the native adapter when Platform.OS is ios', () => {
    const { adapter, expected } = loadAdapterForOS('ios')
    expect(adapter).toBe(expected)
  })

  it('returns the native adapter when Platform.OS is android', () => {
    const { adapter, expected } = loadAdapterForOS('android')
    expect(adapter).toBe(expected)
  })
})

describe('web session audio adapter', () => {
  let webAudioMocks: ReturnType<typeof setupWebAudioMocks>
  let originalWindow: unknown
  let originalNavigator: unknown
  let originalAudioContext: unknown

  beforeEach(() => {
    originalWindow = (global as any).window
    originalNavigator = (global as any).navigator
    originalAudioContext = (global as any).AudioContext
    webAudioMocks = setupWebAudioMocks()
  })

  afterEach(() => {
    ;(global as any).window = originalWindow
    ;(global as any).navigator = originalNavigator
    ;(global as any).AudioContext = originalAudioContext
  })

  it('returns an object with the expected interface without loading native', () => {
    jest.resetModules()
    jest.clearAllMocks()
    setPlatformOS('web')

    const { getSessionAudioAdapter } = require('../session-audio') as typeof import('../session-audio')
    const adapter = getSessionAudioAdapter()

    expect(adapter.initialize).toBeDefined()
    expect(adapter.teardown).toBeDefined()
    expect(adapter.startRecording).toBeDefined()
    expect(adapter.stopRecording).toBeDefined()
    expect(adapter.playPCMData).toBeDefined()
    expect(adapter.setMuted).toBeDefined()

    // Native implementation should not be invoked just by selecting the adapter.
    expect(mockInitialize).not.toHaveBeenCalled()
    expect(mockToggleRecording).not.toHaveBeenCalled()
    expect(mockTearDown).not.toHaveBeenCalled()
    expect(mockPlayPCMData).not.toHaveBeenCalled()
  })

  it('exposes all SessionAudioAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('web')

    await expect(adapter.initialize()).resolves.toBeUndefined()
    expect(() => adapter.teardown()).not.toThrow()
    expect(() => adapter.startRecording()).not.toThrow()
    expect(() => adapter.stopRecording()).not.toThrow()
    expect(() => adapter.playPCMData(new Uint8Array([1, 2, 3]))).not.toThrow()
    expect(() => adapter.setMuted(true)).not.toThrow()
  })

  it('initializes a Web Audio AudioContext', async () => {
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize()

    expect(webAudioMocks.contexts).toHaveLength(1)
    expect(webAudioMocks.contexts[0].close).not.toHaveBeenCalled()
  })

  it('starts recording by requesting the microphone and wiring the audio graph', async () => {
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize()
    await adapter.startRecording()

    expect(webAudioMocks.getUserMedia).toHaveBeenCalledWith({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    })

    const ctx = webAudioMocks.contexts[0]
    expect(ctx.createMediaStreamSource).toHaveBeenCalled()
    expect(ctx.createScriptProcessor).toHaveBeenCalledWith(4096, 1, 1)
    expect(ctx.createGain).toHaveBeenCalled()
  })

  it('delivers resampled microphone PCM and input volume via callbacks', async () => {
    const onMicrophoneData = jest.fn()
    const onInputVolumeLevel = jest.fn()
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize({ onMicrophoneData, onInputVolumeLevel, onOutputVolumeLevel: jest.fn() })
    await adapter.startRecording()

    const ctx = webAudioMocks.contexts[0]
    const processor = (ctx.createScriptProcessor as jest.Mock).mock.results[0].value as {
      onaudioprocess: ((e: { inputBuffer: { getChannelData: (channel: number) => Float32Array } }) => void) | null
      disconnect: jest.Mock
    }

    const input = new Float32Array(4096).fill(0.5)
    processor.onaudioprocess?.({ inputBuffer: { getChannelData: () => input } })

    expect(onInputVolumeLevel).toHaveBeenCalledWith(expect.any(Number))
    expect(onMicrophoneData).toHaveBeenCalledTimes(1)
    const microphoneData = onMicrophoneData.mock.calls[0][0]
    expect(typeof microphoneData).toBe('object')
    expect(typeof (microphoneData as ArrayBuffer).byteLength).toBe('number')
    expect((microphoneData as ArrayBuffer).byteLength).toBeGreaterThan(0)
  })

  it('does not emit microphone data while muted', async () => {
    const onMicrophoneData = jest.fn()
    const onInputVolumeLevel = jest.fn()
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize({ onMicrophoneData, onInputVolumeLevel, onOutputVolumeLevel: jest.fn() })
    await adapter.startRecording()

    const ctx = webAudioMocks.contexts[0]
    const processor = (ctx.createScriptProcessor as jest.Mock).mock.results[0].value as {
      onaudioprocess: ((e: { inputBuffer: { getChannelData: (channel: number) => Float32Array } }) => void) | null
      disconnect: jest.Mock
    }

    adapter.setMuted(true)
    processor.onaudioprocess?.({ inputBuffer: { getChannelData: () => new Float32Array(4096).fill(0.5) } })

    expect(onMicrophoneData).not.toHaveBeenCalled()
    expect(onInputVolumeLevel).not.toHaveBeenCalled()
  })

  it('stops recording and tears down the audio graph', async () => {
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize()
    await adapter.startRecording()

    const ctx = webAudioMocks.contexts[0]
    const processor = (ctx.createScriptProcessor as jest.Mock).mock.results[0].value as {
      onaudioprocess: ((e: { inputBuffer: { getChannelData: (channel: number) => Float32Array } }) => void) | null
      disconnect: jest.Mock
    }

    adapter.stopRecording()

    expect(processor.disconnect).toHaveBeenCalled()
    expect(webAudioMocks.trackStop).toHaveBeenCalled()
  })

  it('plays PCM audio and reports output volume', async () => {
    const onOutputVolumeLevel = jest.fn()
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize({ onMicrophoneData: jest.fn(), onInputVolumeLevel: jest.fn(), onOutputVolumeLevel })

    // 16-bit sample value of 128 -> small non-zero volume.
    adapter.playPCMData(new Uint8Array([0, 128]))

    const ctx = webAudioMocks.contexts[0]
    expect(ctx.createBuffer).toHaveBeenCalledWith(1, 1, 16000)
    expect(ctx.createBufferSource).toHaveBeenCalled()

    const source = (ctx.createBufferSource as jest.Mock).mock.results[0].value as { start: jest.Mock }
    expect(source.start).toHaveBeenCalled()
    expect(onOutputVolumeLevel).toHaveBeenCalledWith(expect.any(Number))
  })

  it('closes the AudioContext and stops active sources on teardown', async () => {
    const { adapter } = loadAdapterForOS('web')

    await adapter.initialize()
    adapter.playPCMData(new Uint8Array([0, 128]))

    const ctx = webAudioMocks.contexts[0]
    const source = (ctx.createBufferSource as jest.Mock).mock.results[0].value as { stop: jest.Mock }

    adapter.teardown()

    expect(source.stop).toHaveBeenCalled()
    expect(ctx.close).toHaveBeenCalled()
  })
})

describe('native session audio adapter', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockInitialize.mockResolvedValue(true)
  })

  it('exposes all SessionAudioAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('ios')

    expect(adapter.initialize).toBeDefined()
    expect(adapter.teardown).toBeDefined()
    expect(adapter.startRecording).toBeDefined()
    expect(adapter.stopRecording).toBeDefined()
    expect(adapter.playPCMData).toBeDefined()
    expect(adapter.setMuted).toBeDefined()

    await expect(adapter.initialize()).resolves.toBeUndefined()
    expect(mockInitialize).toHaveBeenCalledTimes(1)
  })

  it('delegates initialize to the native module', async () => {
    const { adapter } = loadAdapterForOS('ios')
    await adapter.initialize()
    expect(mockInitialize).toHaveBeenCalledTimes(1)
  })

  it('retries initialize once after a teardown when the native module reports failure', async () => {
    const { adapter } = loadAdapterForOS('ios')
    mockInitialize.mockResolvedValueOnce(false).mockResolvedValueOnce(true)

    await adapter.initialize()

    expect(mockInitialize).toHaveBeenCalledTimes(2)
    expect(mockTearDown).toHaveBeenCalledTimes(1)

    adapter.startRecording()
    expect(mockToggleRecording).toHaveBeenCalledWith(true)
  })

  it('stays uninitialized when the native module keeps failing', async () => {
    jest.spyOn(console, 'error').mockImplementation(() => { })
    const { adapter } = loadAdapterForOS('ios')
    mockInitialize.mockResolvedValue(false)

    await adapter.initialize()

    expect(mockInitialize).toHaveBeenCalledTimes(2)
    adapter.startRecording()
    expect(mockToggleRecording).not.toHaveBeenCalled()
  })

  it('delegates playPCMData to the native module', () => {
    const { adapter } = loadAdapterForOS('ios')
    const data = new Uint8Array([1, 2, 3])
    adapter.playPCMData(data)
    expect(mockPlayPCMData).toHaveBeenCalledWith(data)
  })

  it('delegates stopPlayback to the native module', () => {
    const { adapter } = loadAdapterForOS('ios')
    adapter.stopPlayback()
    expect(mockStopPlayback).toHaveBeenCalledTimes(1)
  })

  it('controls recording via toggleRecording', async () => {
    const { adapter } = loadAdapterForOS('ios')
    await adapter.initialize()

    adapter.startRecording()
    expect(mockToggleRecording).toHaveBeenCalledWith(true)

    adapter.stopRecording()
    expect(mockToggleRecording).toHaveBeenCalledWith(false)
  })

  it('tears down by stopping recording and calling tearDown', () => {
    const { adapter } = loadAdapterForOS('ios')

    adapter.teardown()
    expect(mockToggleRecording).toHaveBeenCalledWith(false)
    expect(mockTearDown).toHaveBeenCalledTimes(1)
  })

  it('does not start recording while muted', async () => {
    const { adapter } = loadAdapterForOS('ios')
    await adapter.initialize()

    adapter.setMuted(true)
    expect(mockToggleRecording).toHaveBeenLastCalledWith(false)
    mockToggleRecording.mockClear()

    adapter.startRecording()
    expect(mockToggleRecording).not.toHaveBeenCalled()
  })

  it('unmutes by resuming recording', async () => {
    const { adapter } = loadAdapterForOS('ios')
    await adapter.initialize()

    adapter.setMuted(true)
    adapter.setMuted(false)
    expect(mockToggleRecording).toHaveBeenLastCalledWith(true)
  })

  it('does not call toggleRecording via setMuted before initialize', () => {
    const { adapter } = loadAdapterForOS('ios')

    adapter.setMuted(true)
    expect(mockToggleRecording).not.toHaveBeenCalled()

    adapter.setMuted(false)
    expect(mockToggleRecording).not.toHaveBeenCalled()
  })
})
