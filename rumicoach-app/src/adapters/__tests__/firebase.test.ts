import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform } from 'react-native'

jest.mock('@react-native-firebase/analytics', () => ({
  __esModule: true,
  default: jest.fn(() => ({
    logEvent: jest.fn(),
    setUserId: jest.fn(),
    logScreenView: jest.fn(),
  })),
}))

jest.mock('@react-native-firebase/crashlytics', () => ({
  __esModule: true,
  default: jest.fn(() => ({
    setUserId: jest.fn(),
    recordError: jest.fn(),
  })),
}))

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

const loadAdapterForOS = (os: 'web' | 'ios' | 'android') => {
  jest.resetModules()
  setPlatformOS(os)

  const { getFirebaseAdapter } = require('../firebase') as typeof import('../firebase')
  const { default: expected } = require(
    os === 'web' ? '../firebase/web' : '../firebase/native'
  ) as typeof import('../firebase/web')

  return { adapter: getFirebaseAdapter(), expected }
}

describe('getFirebaseAdapter platform selector', () => {
  beforeEach(() => {
    jest.resetModules()
  })

  it('returns the web no-op adapter when Platform.OS is web', () => {
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

describe('web firebase adapter', () => {
  it('exposes all FirebaseAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('web')
    const error = new Error('web crash')
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})

    expect(adapter.logEvent).toBeDefined()
    expect(adapter.setUserId).toBeDefined()
    expect(adapter.trackScreenView).toBeDefined()
    expect(adapter.setCrashlyticsUserId).toBeDefined()
    expect(adapter.logCrashlyticsError).toBeDefined()

    await expect(adapter.logEvent('test_event')).resolves.toBeUndefined()
    await expect(adapter.setUserId('user_123')).resolves.toBeUndefined()
    await expect(adapter.trackScreenView('TestScreen')).resolves.toBeUndefined()
    expect(() => adapter.setCrashlyticsUserId('user_123')).not.toThrow()
    expect(() => adapter.logCrashlyticsError(error)).not.toThrow()
    expect(consoleErrorSpy).toHaveBeenCalledWith('[Web Crashlytics]', error)

    consoleErrorSpy.mockRestore()
  })
})

describe('native firebase adapter', () => {
  it('exposes all FirebaseAdapter methods and does not throw', async () => {
    const { adapter } = loadAdapterForOS('ios')
    const error = new Error('native crash')

    expect(adapter.logEvent).toBeDefined()
    expect(adapter.setUserId).toBeDefined()
    expect(adapter.trackScreenView).toBeDefined()
    expect(adapter.setCrashlyticsUserId).toBeDefined()
    expect(adapter.logCrashlyticsError).toBeDefined()

    await expect(adapter.logEvent('test_event')).resolves.toBeUndefined()
    await expect(adapter.setUserId('user_123')).resolves.toBeUndefined()
    await expect(adapter.trackScreenView('TestScreen')).resolves.toBeUndefined()
    expect(() => adapter.setCrashlyticsUserId('user_123')).not.toThrow()
    expect(() => adapter.logCrashlyticsError(error)).not.toThrow()
  })
})
