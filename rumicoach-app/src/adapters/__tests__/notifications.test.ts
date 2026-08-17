import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform } from 'react-native'

const mockOnMessage = jest.fn(() => jest.fn())
const mockSetBackgroundMessageHandler = jest.fn()
const mockRequestPermission = jest.fn<() => Promise<number>>()
const mockRegisterDeviceForRemoteMessages = jest.fn<() => Promise<void>>()
const mockGetToken = jest.fn<() => Promise<string | null>>()

jest.mock('@react-native-firebase/messaging', () => {
  const AuthorizationStatus = {
    AUTHORIZED: 1,
    PROVISIONAL: 2,
    DENIED: 0,
    NOT_DETERMINED: -1,
  }

  return {
    __esModule: true,
    default: Object.assign(
      jest.fn(() => ({
        requestPermission: mockRequestPermission,
        registerDeviceForRemoteMessages: mockRegisterDeviceForRemoteMessages,
        getToken: mockGetToken,
        setBackgroundMessageHandler: mockSetBackgroundMessageHandler,
        onMessage: mockOnMessage,
      })),
      { AuthorizationStatus }
    ),
    AuthorizationStatus,
  }
})

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

const loadAdapterForOS = (os: 'web' | 'ios' | 'android') => {
  jest.resetModules()
  setPlatformOS(os)

  const { getNotificationsAdapter } = require('../notifications') as typeof import('../notifications')
  const { default: expected } = require(
    os === 'web' ? '../notifications/web' : '../notifications/native'
  ) as typeof import('../notifications/web')

  return { adapter: getNotificationsAdapter(), expected }
}

describe('getNotificationsAdapter platform selector', () => {
  beforeEach(() => {
    jest.resetModules()
  })

  it('returns the web stub adapter when Platform.OS is web', () => {
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

describe('web notifications adapter', () => {
  it('exposes all NotificationsAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('web')

    expect(adapter.requestPermission).toBeDefined()
    expect(adapter.getToken).toBeDefined()
    expect(adapter.registerBackgroundHandler).toBeDefined()
    expect(adapter.registerForegroundHandler).toBeDefined()

    await expect(adapter.requestPermission()).resolves.toBe(false)
    await expect(adapter.getToken()).resolves.toBeNull()
    expect(() => adapter.registerBackgroundHandler(async () => {})).not.toThrow()
    expect(() => adapter.registerForegroundHandler(() => {})).not.toThrow()
  })
})

describe('native notifications adapter', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockRequestPermission.mockResolvedValue(1)
    mockRegisterDeviceForRemoteMessages.mockResolvedValue(undefined)
    mockGetToken.mockResolvedValue('mock-token')
  })

  it('exposes all NotificationsAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('ios')

    expect(adapter.requestPermission).toBeDefined()
    expect(adapter.getToken).toBeDefined()
    expect(adapter.registerBackgroundHandler).toBeDefined()
    expect(adapter.registerForegroundHandler).toBeDefined()

    await expect(adapter.requestPermission()).resolves.toBe(true)
    await expect(adapter.getToken()).resolves.toBe('mock-token')
    expect(() => adapter.registerBackgroundHandler(async () => {})).not.toThrow()
    expect(() => adapter.registerForegroundHandler(() => {})).not.toThrow()
  })
})
