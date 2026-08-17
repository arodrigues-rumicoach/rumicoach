import axios from 'axios'
import { Platform } from 'react-native'
import Constants from 'expo-constants'
import { getStorageAdapter } from '../adapters/storage'
import { AppEvents } from '../utils/AppEvents'
import { authBackendUrl, regionBackendUrl } from './backend-url'
import { getRegionFromToken } from './jwt'

const TOKEN_KEY = 'rumi_auth_token'
const REFRESH_TOKEN_KEY = 'rumi_refresh_token'

let refreshPromise: Promise<boolean> | null = null

const clearAuthTokens = async () => {
  await getStorageAdapter().deleteItemAsync(TOKEN_KEY)
  await getStorageAdapter().deleteItemAsync(REFRESH_TOKEN_KEY)
}

const refreshAccessToken = async (): Promise<boolean> => {
  const refreshToken = await getStorageAdapter().getItemAsync(REFRESH_TOKEN_KEY)
  if (__DEV__) console.log('[AUTH] refreshAccessToken refreshTokenPresent=', !!refreshToken)
  if (!refreshToken) {
    await clearAuthTokens()
    AppEvents.emit('auth:invalid')
    return false
  }

  try {
    if (__DEV__) console.log(`[AUTH] POST ${authBackendUrl}/auth/refresh`)
    const { data } = await axios.post(`${authBackendUrl}/auth/refresh`, {
      refreshToken,
    }, { timeout: 15000 })

    if (!data.accessToken) {
      if (__DEV__) console.error('[AUTH] refreshAccessToken no accessToken in response')
      await clearAuthTokens()
      AppEvents.emit('auth:invalid')
      return false
    }

    await getStorageAdapter().setItemAsync(TOKEN_KEY, data.accessToken)
    if (data.refreshToken) {
      await getStorageAdapter().setItemAsync(REFRESH_TOKEN_KEY, data.refreshToken)
    }
    if (__DEV__) console.log('[AUTH] refreshAccessToken success')
    return true
  } catch (e: any) {
    if (__DEV__) console.error('[AUTH] refreshAccessToken failed:', e)

    const status = e.response?.status

    // Auth backend explicitly rejected the refresh token (invalid / expired).
    if (status === 400 || status === 401 || status === 403) {
      if (__DEV__) console.log(`[AUTH] refresh token rejected (${status}), clearing tokens`)
      await clearAuthTokens()
      AppEvents.emit('auth:invalid')
      return false
    }

    // Server error (5xx) or transient network failure — keep tokens so the
    // next request can attempt another refresh instead of logging the user out.
    if (__DEV__) console.log(`[AUTH] refresh transient failure (status=${status ?? 'NETWORK'}), keeping tokens`)
    return false
  }
}

const getRefreshPromise = (): Promise<boolean> => {
  if (!refreshPromise) {
    refreshPromise = refreshAccessToken().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

// The server falls back to these headers whenever a request carries no richer
// context of its own, so sending the version on every call improves any report
// that reaches support — not just the feedback form.
const APP_VERSION = Constants.expoConfig?.version ?? 'unknown'

export const api = axios.create({
  baseURL: regionBackendUrl('eu'),
  timeout: 15000,
  headers: {
    'Content-Type': 'application/json',
    'X-Platform': Platform.OS,
    'X-App-Version': APP_VERSION,
  },
})

if (__DEV__) {
  console.log(`[API] authBackendUrl: ${authBackendUrl}`)
  console.log(`[API] regionBackendUrl eu: ${regionBackendUrl('eu')}`)
  console.log(`[API] regionBackendUrl us: ${regionBackendUrl('us')}`)
}

api.interceptors.request.use(async (config) => {
  const token = await getStorageAdapter().getItemAsync(TOKEN_KEY)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
    const region = getRegionFromToken(token)
    config.baseURL = regionBackendUrl(region)
  }
  return config
})

api.interceptors.request.use((config) => {
  if (__DEV__) {
    console.log(`[API] ${config.method?.toUpperCase()} ${config.baseURL}${config.url}`)
    if (config.data) {
      console.log(`[API] BODY ${config.method?.toUpperCase()} ${config.url}:`, config.data)
    }
  }
  return config
})

api.interceptors.response.use(
  (response) => {
    if (__DEV__) {
      console.log(
        `[API] ${response.status} ${response.config.method?.toUpperCase()} ${response.config.url}`
      )
      console.log(`[API] RESPONSE ${response.config.url}:`, response.data)
    }
    return response
  },
  (error) => {
    const status = error.response?.status || 'NETWORK'
    const method = error.config?.method?.toUpperCase() || '?'
    const url = error.config?.url || '?'
    console.error(`[API] ERROR ${status} ${method} ${url} — ${error.message}`, error.response?.data)
    return Promise.reject(error)
  }
)

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401 && !error.config.__isRetry) {
      if (__DEV__) console.log(`[API] 401 on ${error.config?.url} — attempting token refresh`)
      error.config.__isRetry = true
      const success = await getRefreshPromise()
      if (!success) {
        if (__DEV__) console.log('[API] token refresh failed, rejecting request')
        return Promise.reject(error)
      }

      const token = await getStorageAdapter().getItemAsync(TOKEN_KEY)
      if (!token) {
        if (__DEV__) console.log('[API] no token after refresh, rejecting request')
        return Promise.reject(error)
      }

      const region = getRegionFromToken(token)
      error.config.baseURL = regionBackendUrl(region)
      error.config.headers.Authorization = `Bearer ${token}`
      if (__DEV__) console.log(`[API] retrying ${error.config.method?.toUpperCase()} ${error.config.url} with region=${region}`)
      return api.request(error.config)
    }
    return Promise.reject(error)
  }
)

export async function fetchUsageCalendar(month: string) {
  const { data } = await api.get('/usage-calendar', {
    params: { month }
  })
  return data
}
