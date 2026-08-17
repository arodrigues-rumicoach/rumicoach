import axios from 'axios'
import { getStorageAdapter } from '../../adapters/storage'
import { authBackendUrl } from '../backend-url'
import type { AuthApi, AuthCredentials, AuthTokens, AppleAuthPayload, GoogleAuthPayload, RegisterPayload } from './types'

const TOKEN_KEY = 'rumi_auth_token'

const backend = axios.create({
  baseURL: authBackendUrl,
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})

backend.interceptors.request.use((config) => {
  if (__DEV__) {
    console.log(`[AUTH] ${config.method?.toUpperCase()} ${config.baseURL}${config.url}`)
  }
  return config
})

backend.interceptors.response.use(
  (response) => {
    if (__DEV__) {
      console.log(
        `[AUTH] ${response.status} ${response.config.method?.toUpperCase()} ${response.config.url}`
      )
    }
    return response
  },
  (error) => {
    if (__DEV__) {
      const status = error.response?.status || 'NETWORK'
      const method = error.config?.method?.toUpperCase() || '?'
      const url = error.config?.url || '?'
      console.error(
        `[AUTH] ERROR ${status} ${method} ${url} — ${error.message}`,
        error.response?.data
      )
    }
    return Promise.reject(error)
  }
)

const nativeAuthApi: AuthApi = {
  requestVerificationCode: async (type, identifier) => {
    // This path only serves the sign-in flow; the event lets the backend skip
    // sending a code when no account exists (with an identical response either
    // way, so account existence can't be probed).
    const payload: { type: 'email' | 'phone'; event: 'login'; email?: string; phoneNumber?: string } = { type, event: 'login' }
    if (type === 'email') payload.email = identifier
    if (type === 'phone') payload.phoneNumber = identifier

    const { data } = await backend.post('/auth/verifications/request', payload)
    return data.verificationId
  },

  requestVerificationCodeWithIdentifier: async (type, identifier, event) => {
    const payload: { type: 'email' | 'phone'; event: string; email?: string; phoneNumber?: string } = { type, event }
    if (type === 'email') payload.email = identifier
    if (type === 'phone') payload.phoneNumber = identifier

    const { data } = await backend.post('/auth/verifications/request', payload)
    return data.verificationId
  },

  loginWithCode: async ({ type, identifier, code }: AuthCredentials) => {
    const { data } = await backend.post('/auth/login/code', { type, identifier, code })
    return data as AuthTokens
  },

  register: async (data: RegisterPayload) => {
    const { data: responseData } = await backend.post('/auth/register', data)
    return responseData as AuthTokens
  },

  loginWithGoogle: async ({ accessToken }: GoogleAuthPayload) => {
    const { data } = await backend.post('/auth/google', { accessToken })
    return data as AuthTokens
  },

  loginWithApple: async ({ identityToken, email, name }: AppleAuthPayload) => {
    const { data } = await backend.post('/auth/apple', { identityToken, email, name })
    return data as AuthTokens
  },

  linkAppleAccount: async (identityToken: string) => {
    const token = await getStorageAdapter().getItemAsync(TOKEN_KEY)
    const headers: Record<string, string> = {}
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }
    await backend.post('/auth/me/link/apple', { identityToken }, { headers })
  },

  logout: async () => {
    // Native logout is a no-op at the API layer; the caller clears SecureStore.
  },

  verifyAndUpdateIdentifier: async (type, identifier, verificationId) => {
    const token = await getStorageAdapter().getItemAsync(TOKEN_KEY)
    const headers: Record<string, string> = {}
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }
    const { data } = await backend.put('/auth/me/identifier', { type, identifier, verificationId }, { headers })
    return data
  },

  verifyCode: async (type, identifier, code) => {
    const payload: { type: 'email' | 'phone'; code: string; email?: string; phoneNumber?: string } = { type, code }
    if (type === 'email') payload.email = identifier
    if (type === 'phone') payload.phoneNumber = identifier
    await backend.post('/auth/verifications/verify', payload)
  },

  getSessionToken: async () => {
    return getStorageAdapter().getItemAsync(TOKEN_KEY)
  },
}

export default nativeAuthApi
