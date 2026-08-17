import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform , View } from 'react-native'
import { render, waitFor, act } from '@testing-library/react-native'
import { useContext } from 'react'
import { I18nProvider } from '../../i18n'

jest.mock('expo-router', () => ({
  router: { replace: jest.fn() },
}))

jest.mock('../../api', () => ({
  api: { get: jest.fn(), defaults: { headers: { common: {} } } },
}))

jest.mock('../../api/auth', () => {
  const mockAuthApi = {
    loginWithGoogle: jest.fn(),
    loginWithCode: jest.fn(),
    register: jest.fn(),
    getSessionToken: jest.fn(),
    logout: jest.fn(),
  }
  return {
    getAuthApi: jest.fn(() => mockAuthApi),
    __mockAuthApi: mockAuthApi,
  }
})

jest.mock('../../adapters/storage', () => {
  const mockStorage = {
    getItemAsync: jest.fn(),
    setItemAsync: jest.fn(),
    deleteItemAsync: jest.fn(),
  }
  return {
    getStorageAdapter: jest.fn(() => mockStorage),
    __mockStorage: mockStorage,
  }
})

jest.mock('../../adapters/firebase', () => {
  const mockFirebase = {
    logEvent: jest.fn(),
    setUserId: jest.fn(),
    trackScreenView: jest.fn(),
    setCrashlyticsUserId: jest.fn(),
    logCrashlyticsError: jest.fn(),
  }
  return {
    getFirebaseAdapter: jest.fn(() => mockFirebase),
    __mockFirebase: mockFirebase,
  }
})

jest.mock('../../adapters/notifications', () => {
  const mockNotifications = {
    requestPermission: jest.fn(),
    getToken: jest.fn(),
    registerToken: jest.fn(),
    registerBackgroundHandler: jest.fn(),
    registerForegroundHandler: jest.fn(),
  }
  return {
    getNotificationsAdapter: jest.fn(() => mockNotifications),
    __mockNotifications: mockNotifications,
  }
})

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

describe('AuthProvider', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders without crashing on web', async () => {
    setPlatformOS('web')
    const { AuthProvider } = require('../AuthContext')
    const { getByTestId } = await render(
      <I18nProvider>
        <AuthProvider>
          <View testID="child" />
        </AuthProvider>
      </I18nProvider>,
    )
    expect(getByTestId('child')).toBeTruthy()
  })

  it('renders without crashing on ios', async () => {
    setPlatformOS('ios')
    const { AuthProvider } = require('../AuthContext')
    const { getByTestId } = await render(
      <I18nProvider>
        <AuthProvider>
          <View testID="child" />
        </AuthProvider>
      </I18nProvider>,
    )
    expect(getByTestId('child')).toBeTruthy()
  })

  it('renders without crashing on android', async () => {
    setPlatformOS('android')
    const { AuthProvider } = require('../AuthContext')
    const { getByTestId } = await render(
      <I18nProvider>
        <AuthProvider>
          <View testID="child" />
        </AuthProvider>
      </I18nProvider>,
    )
    expect(getByTestId('child')).toBeTruthy()
  })

  // Regression: on Android, SecureStore sometimes returned null from a read issued
  // immediately after the write it had just awaited. The login paths re-read the
  // token they had only just persisted and silently `return`ed when that read came
  // back empty, so setUser() never ran. The user was authenticated — the token was
  // in storage, and relaunching the app landed them on Journey — but nothing in the
  // tree knew it, so they sat on the sign-in screen with no error shown.
  //
  // getSessionToken is forced to null here to stand in for that flaky read. The user
  // must still be set: the token we just received is the one that matters.
  it.each([
    ['loginWithGoogle', (auth: any) => auth.loginWithGoogle('google-access-token')],
    ['loginWithVerificationCode', (auth: any) => auth.loginWithVerificationCode('email', 'a@b.co', '123456')],
  ])('%s sets the user even when the token read-back comes back empty', async (_name, run) => {
    setPlatformOS('android')
    const { api } = require('../../api')
    const { __mockAuthApi } = require('../../api/auth')

    __mockAuthApi.loginWithGoogle.mockResolvedValue({ accessToken: 'server-token', refreshToken: 'refresh' })
    __mockAuthApi.loginWithCode.mockResolvedValue({ accessToken: 'server-token', refreshToken: 'refresh' })
    __mockAuthApi.getSessionToken.mockResolvedValue(null) // the flaky read
    api.get.mockResolvedValue({ data: { id: 'user-1', name: 'Armando' } })

    const { AuthProvider, AuthContext } = require('../AuthContext')
    let auth: any
    function Probe() {
      auth = useContext(AuthContext)
      return <View testID="child" />
    }

    render(
      <I18nProvider>
        <AuthProvider><Probe /></AuthProvider>
      </I18nProvider>,
    )

    await waitFor(() => expect(auth).toBeTruthy())
    await act(async () => { await run(auth) })

    await waitFor(() => expect(auth.user).toEqual({ id: 'user-1', name: 'Armando' }))
  })
})
