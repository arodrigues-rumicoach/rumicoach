import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { renderHook, act } from '@testing-library/react-native'

jest.mock('expo-router', () => ({
  router: { replace: jest.fn(), push: jest.fn() },
}))

jest.mock('expo-localization', () => ({
  getLocales: jest.fn(() => [{ regionCode: 'PT' }]),
}))

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

const mockUpdateUser = jest.fn().mockResolvedValue(undefined)
const mockEnsureValidToken = jest.fn().mockResolvedValue('test-token')
const mockRegister = jest.fn().mockResolvedValue(undefined)

jest.mock('../../hooks/useAuth', () => ({
  useAuth: jest.fn(() => ({
    user: null,
    token: null,
    isLoading: false,
    register: mockRegister,
    updateUser: mockUpdateUser,
    ensureValidToken: mockEnsureValidToken,
  })),
}))

jest.mock('../../adapters/auth/useGoogleSignIn', () => ({
  useGoogleSignIn: jest.fn(() => jest.fn().mockResolvedValue({
    accessToken: 'google-access-token',
    idToken: 'google-id-token',
    profile: { email: 'test@gmail.com', name: 'Test User' },
  })),
}))

jest.mock('../../api/errors', () => ({
  messageForApiError: jest.fn((e: any, fallback: string) => fallback),
  parseApiError: jest.fn(() => ({ code: '' })),
}))

jest.mock('../../utils/validation', () => ({
  isValidEmail: jest.fn((email: string) => email.includes('@')),
  isValidPhone: jest.fn((phone: string) => phone.length >= 6),
}))

jest.mock('../../api/backend-url', () => ({
  authBackendUrl: 'https://auth.test.rumi.coach',
}))

jest.mock('../../utils/countries', () => ({
  COUNTRIES: [
    { code: 'PT', phoneCode: '+351', name: 'Portugal' },
    { code: 'US', phoneCode: '+1', name: 'United States' },
  ],
}))

jest.mock('../../i18n', () => {
  const mockI18n = {
    t: jest.fn((key: string, fallback?: string) => fallback || key),
    locale: 'en-US',
  }
  return { __esModule: true, default: mockI18n }
})

const mockFetch = jest.fn()
global.fetch = mockFetch

const setupEmailFlow = async (result: any) => {
  act(() => result.current.setName('Test User'))
  act(() => result.current.handleNext()) // NAME → METHOD

  act(() => result.current.setSignupMethod('email'))
  act(() => result.current.setEmail('test@example.com'))

  mockFetch.mockResolvedValueOnce({
    ok: true,
    json: () => Promise.resolve({ verificationId: 'test-id' }),
  })

  await act(async () => {
    await result.current.handleNext() // METHOD → VERIFY
  })

  mockFetch.mockResolvedValueOnce({
    ok: true,
    json: () => Promise.resolve({}),
  })
  act(() => result.current.setVerificationCode('123456'))
  await act(async () => {
    await result.current.handleNext() // VERIFY (verify code)
  })

  act(() => result.current.handleNext()) // VERIFY → REGION_TERMS
}

describe('useSignupForm - back navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    })
  })

  it('back from REGION_TERMS goes to METHOD with clean state', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    await setupEmailFlow(result)
    expect(result.current.step).toBe('REGION_TERMS')
    expect(result.current.signupMethod).toBe('email')

    act(() => result.current.handleBack())

    expect(result.current.step).toBe('METHOD')
    expect(result.current.signupMethod).toBeNull()
    expect(result.current.codeSent).toBe(false)
    expect(result.current.isVerified).toBe(false)
    expect(result.current.verificationId).toBe('')
    expect(result.current.verificationCode).toBe('')
    expect(result.current.countdown).toBe(0)
  })

  it('back from METHOD goes to NAME when no method selected', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    act(() => result.current.setName('Test User'))
    act(() => result.current.handleNext())
    expect(result.current.step).toBe('METHOD')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('NAME')
  })

  it('back from METHOD stays on METHOD when method is selected', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    act(() => result.current.setName('Test User'))
    act(() => result.current.handleNext())
    expect(result.current.step).toBe('METHOD')

    act(() => result.current.setSignupMethod('email'))
    expect(result.current.signupMethod).toBe('email')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('METHOD')
    expect(result.current.signupMethod).toBeNull()
  })

  it('back from VERIFY goes to METHOD with clean state', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    act(() => result.current.setName('Test User'))
    act(() => result.current.handleNext())

    act(() => result.current.setSignupMethod('email'))
    act(() => result.current.setEmail('test@example.com'))

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ verificationId: 'test-id' }),
    })

    await act(async () => {
      await result.current.handleNext()
    })
    expect(result.current.step).toBe('VERIFY')

    act(() => result.current.handleBack())

    expect(result.current.step).toBe('METHOD')
    expect(result.current.signupMethod).toBeNull()
    expect(result.current.codeSent).toBe(false)
    expect(result.current.isVerified).toBe(false)
  })

  it('full round trip: NAME → METHOD → VERIFY → REGION_TERMS → back ×2 → NAME', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    await setupEmailFlow(result)
    expect(result.current.step).toBe('REGION_TERMS')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('METHOD')
    expect(result.current.signupMethod).toBeNull()

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('NAME')
    expect(result.current.name).toBe('Test User')

    // Forward again should work
    act(() => result.current.handleNext())
    expect(result.current.step).toBe('METHOD')
  })

  it('Google flow round trip: METHOD → REGION_TERMS → back → METHOD → NAME', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    act(() => result.current.setName('Test User'))
    act(() => result.current.handleNext())

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({}),
    })

    await act(async () => {
      await result.current.handleGoogleLogin()
    })
    expect(result.current.step).toBe('REGION_TERMS')
    expect(result.current.signupMethod).toBe('google')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('METHOD')
    expect(result.current.signupMethod).toBeNull()

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('NAME')
  })

  it('back from COACH_PREFERENCE goes to REGION_TERMS', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({}),
    })

    act(() => result.current.setName('Test User'))
    act(() => result.current.handleNext())
    await act(async () => {
      await result.current.handleGoogleLogin()
    })

    act(() => result.current.handleFillInManually())
    expect(result.current.step).toBe('PROFILE_DATA')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('COACH_PREFERENCE')
  })

  it('back from PROFILE_DATA goes to COACH_PREFERENCE', async () => {
    const { useSignupForm } = require('../../hooks/useSignupForm')
    const { result } = renderHook(() => useSignupForm())

    act(() => result.current.handleFillInManually())
    expect(result.current.step).toBe('PROFILE_DATA')

    act(() => result.current.handleBack())
    expect(result.current.step).toBe('COACH_PREFERENCE')
  })
})
