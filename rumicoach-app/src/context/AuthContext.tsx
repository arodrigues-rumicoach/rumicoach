import { createContext, useState, useEffect, useCallback, useRef, useMemo, type ReactNode } from 'react'
import { router } from 'expo-router'
import { api, type User, type DataScope } from '../api'
import { getAuthApi } from '../api/auth'
import type { RegisterPayload } from '../api/auth/types'
import { decodeJwt } from '../api/jwt'
import { useI18n } from '../i18n'
import { AppEvents } from '../utils/AppEvents'
import { getStorageAdapter } from '../adapters/storage'
import { getFirebaseAdapter } from '../adapters/firebase'
import { getNotificationsAdapter } from '../adapters/notifications'
import {
  identifyUser,
  resetUser,
  trackLogin,
  trackDataDeleted,
  trackAccountDeleted,
} from '@/analytics'

interface AuthContextType {
  user: User | null
  token: string | null
  isLoading: boolean
  appLanguage: string
  restoreSession: () => Promise<void>
  requestVerificationCode: (type: 'email' | 'phone', identifier: string) => Promise<string>
  loginWithVerificationCode: (type: 'email' | 'phone', identifier: string, code: string) => Promise<void>
  register: (data: RegisterPayload) => Promise<void>
  loginWithGoogle: (idToken: string) => Promise<void>
  loginWithApple: (identityToken: string, email?: string, name?: string) => Promise<void>
  logout: () => Promise<void>
  /** Refetches /me and returns the fresh profile, so a caller that needs an up-to-date
   *  field (the session gate needs the balance) can read it without waiting a render
   *  for the state to land. Returns null when there is no session to refresh. */
  refreshUser: () => Promise<User | null>
  updateUser: (data: Partial<User>) => Promise<void>
  setLanguage: (lang: string) => Promise<void>
  ensureValidToken: () => Promise<string | null>
  deleteUserData: (scope?: DataScope) => Promise<void>
  deleteUserAccount: () => Promise<void>
  isAdmin: boolean
}

const TOKEN_KEY = 'rumi_auth_token'
const REFRESH_TOKEN_KEY = 'rumi_refresh_token'

export const AuthContext = createContext<AuthContextType | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const { locale: i18nLocale, setLanguage: setI18nLanguage } = useI18n()
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [appLanguage, setAppLanguageState] = useState(i18nLocale)
  const [isAdmin, setIsAdmin] = useState(false)

  useEffect(() => {
    setAppLanguageState(i18nLocale)
  }, [i18nLocale])

  const userRef = useRef(user)
  const tokenRef = useRef(token)
  useEffect(() => { userRef.current = user }, [user])
  useEffect(() => { tokenRef.current = token }, [token])

  useEffect(() => {
    if (token) {
      const decoded = decodeJwt(token)
      setIsAdmin(!!decoded?.is_admin)
    } else {
      setIsAdmin(false)
    }
  }, [token])

  const setAppLanguage = useCallback(async (lang: string) => {
    setAppLanguageState(lang)
    setI18nLanguage(lang)
  }, [setI18nLanguage])

  const clearStoredTokens = useCallback(async () => {
    await getStorageAdapter().deleteItemAsync(TOKEN_KEY)
    await getStorageAdapter().deleteItemAsync(REFRESH_TOKEN_KEY)
  }, [])

  const persistTokens = useCallback(async (accessToken: string, refreshToken?: string) => {
    await getStorageAdapter().setItemAsync(TOKEN_KEY, accessToken)
    if (refreshToken) {
      await getStorageAdapter().setItemAsync(REFRESH_TOKEN_KEY, refreshToken)
    }
  }, [])

  const restoreSession = useCallback(async () => {
    setIsLoading(true)
    try {
      const currentToken = await getAuthApi().getSessionToken()
      logToken('[AUTH_FLOW] restoreSession token', currentToken)
      if (!currentToken) {
        setToken(null)
        setUser(null)
        return
      }
      setToken(currentToken)
      if (__DEV__) console.log('[AUTH_FLOW] restoreSession fetching /me')
      const { data } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] restoreSession /me success userId=', data.id)
      setUser(data)
      if (data.preferredLanguage) {
        await setAppLanguage(data.preferredLanguage)
      }
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] restoreSession failed:', e)
      setToken(null)
      setUser(null)
      await clearStoredTokens()
    } finally {
      setIsLoading(false)
    }
  }, [clearStoredTokens, setAppLanguage])

  const updateUser = useCallback(async (data: Partial<User>) => {
    try {
      const { data: updated } = await api.patch('/me', data)
      setUser(updated)
    } catch (e) {
      if (__DEV__) console.error('updateUser failed:', e)
      throw e
    }
  }, [])

  const refreshUser = useCallback(async () => {
    try {
      const currentToken = await getAuthApi().getSessionToken()
      logToken('[AUTH_FLOW] refreshUser token', currentToken)
      if (!currentToken) return null
      if (__DEV__) console.log('[AUTH_FLOW] refreshUser fetching /me')
      const { data } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] refreshUser /me success userId=', data.id)
      setUser(data)
      return data as User
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] refreshUser failed:', e)
      throw e
    }
  }, [])

  const initNotifications = useCallback(async () => {
    try {
      const granted = await getNotificationsAdapter().requestPermission()
      if (granted) {
        const pushToken = await getNotificationsAdapter().getToken()
        if (pushToken) {
          await getNotificationsAdapter().registerToken(pushToken)
        }
      }
    } catch (e) {
      if (__DEV__) console.error('[NOTIF] initNotifications error:', e)
    }
  }, [])

  const logout = useCallback(async () => {
    try {
      await getAuthApi().logout()
    } catch (e) {
      if (__DEV__) console.error('Server logout failed:', e)
    } finally {
      setUser(null)
      setToken(null)
      getFirebaseAdapter().setCrashlyticsUserId(null)
      // Without this the next person to sign in on this device inherits the
      // previous one's PostHog timeline and session recordings.
      resetUser()
      await clearStoredTokens()
    }
  }, [clearStoredTokens])

  useEffect(() => {
    restoreSession()
  }, [restoreSession])

  useEffect(() => {
    if (!isLoading && token && !user) {
      refreshUser().catch(() => { })
    }
  }, [isLoading, token, user, refreshUser])

  useEffect(() => {
    if (user) {
      getFirebaseAdapter().setCrashlyticsUserId(user.id)
      getFirebaseAdapter().setUserId(user.id)
      identifyUser(user)
      initNotifications()
    }
  }, [user, initNotifications])

  useEffect(() => {
    const unsubscribe = AppEvents.on('auth:invalid', () => {
      logout()
      router.replace('/(auth)/signin')
    })
    return () => { unsubscribe() }
  }, [logout])

  const logToken = (label: string, token: string | null | undefined) => {
    if (!__DEV__ || !token) return
    console.log(`${label}: ${token.slice(0, 8)}...${token.slice(-8)}`)
  }

  const requestVerificationCode = useCallback(async (type: 'email' | 'phone', identifier: string): Promise<string> => {
    if (__DEV__) console.log(`[AUTH_FLOW] requestVerificationCode type=${type} identifier=${identifier}`)
    return getAuthApi().requestVerificationCode(type, identifier)
  }, [])

  const loginWithVerificationCode = useCallback(async (type: 'email' | 'phone', identifier: string, code: string) => {
    if (__DEV__) console.log(`[AUTH_FLOW] loginWithVerificationCode type=${type} identifier=${identifier}`)
    try {
      const { accessToken, refreshToken } = await getAuthApi().loginWithCode({ type, identifier, code })
      logToken('[AUTH_FLOW] accessToken received', accessToken)
      if (refreshToken) logToken('[AUTH_FLOW] refreshToken received', refreshToken)
      await persistTokens(accessToken, refreshToken)
      setToken(accessToken)
      // No storage read-back — see the note in loginWithGoogle. The same silent
      // `if (!currentToken) return` was here, so this path could strand the user on
      // the sign-in screen in exactly the same way.
      if (__DEV__) console.log('[AUTH_FLOW] fetching /me')
      const { data: userData } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] /me success userId=', userData.id)
      setUser(userData)
      trackLogin(type)
      if (userData.preferredLanguage) {
        await setAppLanguage(userData.preferredLanguage)
      }
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] loginWithVerificationCode failed:', e)
      throw e
    }
  }, [persistTokens, setAppLanguage])

  const register = useCallback(async (data: RegisterPayload) => {
    if (__DEV__) console.log('[AUTH_FLOW] register start', { name: data.name, hasGoogleToken: !!data.googleAccessToken })
    try {
      const { accessToken, refreshToken } = await getAuthApi().register(data)
      logToken('[AUTH_FLOW] accessToken received', accessToken)
      if (refreshToken) logToken('[AUTH_FLOW] refreshToken received', refreshToken)
      await persistTokens(accessToken, refreshToken)
      setToken(accessToken)
      // No storage read-back — see the note in loginWithGoogle. The same silent
      // `if (!currentToken) return` was here, so this path could strand the user on
      // the sign-in screen in exactly the same way.
      if (__DEV__) console.log('[AUTH_FLOW] fetching /me')
      const { data: userData } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] /me success userId=', userData.id)
      setUser(userData)
      if (userData.preferredLanguage) {
        await setAppLanguage(userData.preferredLanguage)
      }
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] register failed:', e)
      throw e
    }
  }, [persistTokens, setAppLanguage])

  const loginWithGoogle = useCallback(async (accessToken: string) => {
    if (__DEV__) console.log('[AUTH_FLOW] loginWithGoogle start tokenPresent=', !!accessToken)
    try {
      const { accessToken: serverAccessToken, refreshToken } = await getAuthApi().loginWithGoogle({ accessToken })
      logToken('[AUTH_FLOW] server accessToken received', serverAccessToken)
      if (refreshToken) logToken('[AUTH_FLOW] refreshToken received', refreshToken)
      await persistTokens(serverAccessToken, refreshToken)
      setToken(serverAccessToken)
      // Deliberately NOT re-reading the token back out of storage here.
      //
      // This used to be `getSessionToken()` followed by `if (!currentToken) return`,
      // and that silent return was the "login works but stays on the sign-in screen"
      // bug on Android: SecureStore occasionally hands back null right after the
      // write it just awaited, so the read-back failed even though the token was
      // safely stored. setUser() never ran, nothing in the tree learned there was a
      // user, and no error surfaced either — it was a return, not a throw. Relaunching
      // fixed it because restoreSession() then read the token that had been there all
      // along.
      //
      // The round-trip proved nothing anyway: serverAccessToken is the value we just
      // wrote. If it were somehow unusable, /me answers 401 and we throw, which the
      // caller shows to the user instead of failing invisibly.
      if (__DEV__) console.log('[AUTH_FLOW] fetching /me')
      const { data: userData } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] /me success userId=', userData.id)
      setUser(userData)
      trackLogin('google')
      if (userData.preferredLanguage) {
        await setAppLanguage(userData.preferredLanguage)
      }
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] loginWithGoogle failed:', e)
      throw e
    }
  }, [persistTokens, setAppLanguage])

  const loginWithApple = useCallback(async (identityToken: string, email?: string, name?: string) => {
    if (__DEV__) console.log('[AUTH_FLOW] loginWithApple start tokenPresent=', !!identityToken)
    try {
      const { accessToken: serverAccessToken, refreshToken } = await getAuthApi().loginWithApple({ identityToken, email, name })
      logToken('[AUTH_FLOW] server accessToken received', serverAccessToken)
      if (refreshToken) logToken('[AUTH_FLOW] refreshToken received', refreshToken)
      await persistTokens(serverAccessToken, refreshToken)
      setToken(serverAccessToken)
      if (__DEV__) console.log('[AUTH_FLOW] fetching /me')
      const { data: userData } = await api.get('/me')
      if (__DEV__) console.log('[AUTH_FLOW] /me success userId=', userData.id)
      setUser(userData)
      trackLogin('apple')
      if (userData.preferredLanguage) {
        await setAppLanguage(userData.preferredLanguage)
      }
    } catch (e) {
      if (__DEV__) console.error('[AUTH_FLOW] loginWithApple failed:', e)
      throw e
    }
  }, [persistTokens, setAppLanguage])

  const setLanguage = useCallback(async (lang: string) => {
    await setAppLanguage(lang)
    if (userRef.current && tokenRef.current) {
      updateUser({ preferredLanguage: lang }).catch(() => { })
    }
  }, [setAppLanguage, updateUser])

  const ensureValidToken = useCallback(async (): Promise<string | null> => {
    return getAuthApi().getSessionToken()
  }, [])

  // Omitting `scope` deletes everything, matching the endpoint's own default —
  // so the pre-scope call sites keep their behaviour without being touched.
  const deleteUserData = useCallback(async (scope?: DataScope) => {
    try {
      await api.delete('/me/data', scope ? { params: { scope } } : undefined)
      trackDataDeleted(scope ?? 'all')
      await refreshUser()
    } catch (e) {
      if (__DEV__) console.error('deleteUserData failed:', e)
      throw e
    }
  }, [refreshUser])

  const deleteUserAccount = useCallback(async () => {
    try {
      await api.delete('/me')
      // Before logout(), which calls reset() — after it the event would be
      // attributed to a fresh anonymous id instead of the account that left.
      trackAccountDeleted()
      await logout()
    } catch (e) {
      if (__DEV__) console.error('deleteUserAccount failed:', e)
      throw e
    }
  }, [logout])

  const value = useMemo(() => ({
    user,
    token,
    isLoading,
    appLanguage,
    isAdmin,
    restoreSession,
    requestVerificationCode,
    loginWithVerificationCode,
    register,
    loginWithGoogle,
    loginWithApple,
    logout,
    refreshUser,
    updateUser,
    setLanguage,
    ensureValidToken,
    deleteUserData,
    deleteUserAccount,
  }), [user, token, isLoading, appLanguage, isAdmin, restoreSession, requestVerificationCode,
    loginWithVerificationCode, register, loginWithGoogle, loginWithApple, logout, refreshUser,
    updateUser, setLanguage, ensureValidToken, deleteUserData, deleteUserAccount])

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  )
}
