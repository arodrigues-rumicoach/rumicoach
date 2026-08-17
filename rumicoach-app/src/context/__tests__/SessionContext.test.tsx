import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform , View } from 'react-native'
import { render, act } from '@testing-library/react-native'

let mockIsWeb = false
let nativeModuleImportCount = 0

jest.mock('../../adapters/platform', () => ({
  get isWeb() {
    return mockIsWeb
  },
}))

jest.mock('expo-audio', () => ({
  setAudioModeAsync: jest.fn(() => Promise.resolve()),
}))

jest.mock('expo-router', () => ({
  router: {
    push: jest.fn(),
    replace: jest.fn(),
    navigate: jest.fn(),
    back: jest.fn(),
  },
}))

jest.mock('@speechmatics/expo-two-way-audio', () => {
  nativeModuleImportCount += 1
  return {
    initialize: jest.fn(() => Promise.resolve()),
    playPCMData: jest.fn(),
    tearDown: jest.fn(),
    toggleRecording: jest.fn(),
    getMicrophonePermissionsAsync: jest.fn(() => Promise.resolve({ granted: true })),
    requestMicrophonePermissionsAsync: jest.fn(() => Promise.resolve({ granted: true })),
    addExpoTwoWayAudioEventListener: jest.fn(() => ({ remove: jest.fn() })),
  }
})

// Stable objects, not fresh ones per call. The provider derives callbacks (disconnect
// among them) from these, and an effect cleans up on disconnect's identity — hand back a
// new function each render and that effect re-runs forever, tearing the session down on
// every pass until the heap gives out.
const mockAuth = {
  token: 'test-token',
  ensureValidToken: jest.fn(() => Promise.resolve('test-token')),
  updateUser: jest.fn(() => Promise.resolve()),
  refreshUser: jest.fn(() => Promise.resolve(null)),
  user: { id: 'u1' },
}

jest.mock('../../hooks/useAuth', () => ({ useAuth: jest.fn(() => mockAuth) }))

// The whole SessionAudioAdapter surface, as one stable object: the provider calls
// getSessionAudioAdapter() afresh at each site, and a mock that minted a new object per
// call would make "did it tear down?" unanswerable. Kept in sync with the interface in
// src/adapters/session-audio — a missing method throws from inside an effect, which
// surfaces as an unrelated render failure.
const mockAudioAdapter = {
  initialize: jest.fn(() => Promise.resolve()),
  teardown: jest.fn(),
  startRecording: jest.fn(),
  stopRecording: jest.fn(),
  playPCMData: jest.fn(),
  stopPlayback: jest.fn(),
  setMuted: jest.fn(),
}

jest.mock('../../adapters/session-audio', () => ({
  getSessionAudioAdapter: jest.fn(() => mockAudioAdapter),
}))

jest.mock('../../api/jwt', () => ({ getRegionFromToken: jest.fn(() => 'eu') }))
jest.mock('../../api/backend-url', () => ({ regionWebSocketUrl: jest.fn(() => 'ws://test/ws/chat') }))
jest.mock('@/analytics', () => ({ trackSessionStarted: jest.fn(), trackSessionEnded: jest.fn() }))

// Stable for the same reason as mockAuth above — setMusicEnabled is one of disconnect's
// dependencies.
const mockAudio = {
  isMusicEnabled: false,
  toggleMusic: jest.fn(),
  setMusicEnabled: jest.fn(),
  fadeOut: jest.fn(),
  fadeIn: jest.fn(),
  pauseAmbient: jest.fn(),
}

jest.mock('../../hooks/useAudio', () => ({ useAudio: jest.fn(() => mockAudio) }))

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
  mockIsWeb = os === 'web'
}

describe('SessionProvider', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    nativeModuleImportCount = 0
    delete (global as any).window
  })

  it('renders on web without importing the native two-way audio module', async () => {
    setPlatformOS('web')
    const { SessionProvider } = require('../SessionContext')

    const { getByTestId } = await render(
      <SessionProvider>
        <View testID="child" />
      </SessionProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(nativeModuleImportCount).toBe(0)
  })

  it('renders on ios', async () => {
    setPlatformOS('ios')
    const { SessionProvider } = require('../SessionContext')

    const { getByTestId } = await render(
      <SessionProvider>
        <View testID="child" />
      </SessionProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
  })

  it('renders on android', async () => {
    setPlatformOS('android')
    const { SessionProvider } = require('../SessionContext')

    const { getByTestId } = await render(
      <SessionProvider>
        <View testID="child" />
      </SessionProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
  })
})

// The app no longer decides whether a session may start — it opens the socket and the
// server answers. These pin the only two forms "no" can arrive in, because the failure
// they replace was silent: the user tapped start and watched nothing happen.
describe('SessionProvider refusal handling', () => {
  let sockets: FakeSocket[]

  class FakeSocket {
    static OPEN = 1
    url: string
    readyState = 1
    binaryType = ''
    onopen: (() => void) | null = null
    onmessage: ((e: { data: string }) => void) | null = null
    onclose: ((e: { code?: number }) => void) | null = null
    onerror: ((e: unknown) => void) | null = null
    close = jest.fn()

    constructor(url: string) {
      this.url = url
      sockets.push(this)
    }
  }

  beforeEach(() => {
    jest.clearAllMocks()
    sockets = []
    setPlatformOS('web')
    ;(global as unknown as { WebSocket: unknown }).WebSocket = FakeSocket
  })

  // Renders the provider and returns its context value, re-read after each act().
  const mountSession = async () => {
    const { SessionProvider, SessionContext } = require('../SessionContext')
    let ctx: any
    const Probe = () => {
      ctx = React.useContext(SessionContext)
      return <View testID="child" />
    }
    await render(
      <SessionProvider>
        <Probe />
      </SessionProvider>,
    )
    return { get value() { return ctx } }
  }

  it('routes to the paywall when the server refuses over the socket', async () => {
    const session = await mountSession()

    await act(async () => { await session.value.connect(false, 'session_vision') })
    const ws = sockets[0]
    expect(ws).toBeTruthy()

    // The refusal: an error frame naming the reason, then the close that follows it.
    await act(async () => {
      ws.onmessage?.({ data: JSON.stringify({ type: 'error', code: 'INSUFFICIENT_BALANCE' }) })
      ws.onclose?.({ code: 4402 })
    })

    // Set after disconnect(), which clears pendingNavigation on its way through — the
    // ordering this assertion exists to hold.
    expect(session.value.pendingNavigation).toBe('paywall')
  })

  // A socket torn down before the error frame is read still has to say why, or the
  // refusal is invisible again.
  it('routes to the paywall on the close code alone', async () => {
    const session = await mountSession()

    await act(async () => { await session.value.connect(false, 'session_vision') })
    await act(async () => { sockets[0].onclose?.({ code: 4402 }) })

    expect(session.value.pendingNavigation).toBe('paywall')
  })

  it('leaves an ordinary close alone', async () => {
    const session = await mountSession()

    await act(async () => { await session.value.connect(false, 'checkin') })
    await act(async () => { sockets[0].onclose?.({ code: 1000 }) })

    expect(session.value.pendingNavigation).toBeNull()
  })
})
