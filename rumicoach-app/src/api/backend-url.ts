/**
 * Multi-region backend URL resolution.
 *
 * Auth endpoints (login, register, google, refresh, verifications) always hit
 * the auth backend. All other authenticated endpoints are routed to the region
 * encoded in the user's access token (default: eu).
 */

export const DEFAULT_REGION = 'eu'

/**
 * The bare API domain — rumi.coach in production, qa.rumi.coach in QA. Every host is built
 * from it by prefixing: `auth.` for the auth backend, the region name for everything else.
 *
 * EXPO_PUBLIC_API is the primary domain setting for QA and PROD.
 * Individual endpoints can be overridden via:
 *   - EXPO_PUBLIC_API_AUTH (or EXPO_PUBLIC_AUTH_BACKEND_URL)
 *   - EXPO_PUBLIC_API_EU / EXPO_PUBLIC_API_US (or EXPO_PUBLIC_{REGION}_BACKEND_URL)
 */
const rawApi = process.env.EXPO_PUBLIC_API?.trim() || ''
const defaultSchema = rawApi.startsWith('http://') ? 'http' : 'https'

const apiDomain =
  rawApi
    .replace(/^https?:\/\//, '')
    .replace(/\/+$/, '')
    .replace(/^(auth|eu|us)\./, '') ||
  (process.env.EXPO_PUBLIC_API_AUTH?.includes('.')
    ? process.env.EXPO_PUBLIC_API_AUTH.replace(/^https?:\/\//, '').replace(/^auth\./, '')
    : '') ||
  'rumi.coach'

const resolveEndpointUrl = (override: string | undefined, defaultHost: string): string => {
  const value = override?.trim()
  if (!value) {
    return `${defaultSchema}://${defaultHost}/v1`
  }
  // If it's already a full URL (http:// or https://)
  if (value.startsWith('http://') || value.startsWith('https://')) {
    const withoutTrailingSlash = value.replace(/\/+$/, '')
    return withoutTrailingSlash.endsWith('/v1') ? withoutTrailingSlash : `${withoutTrailingSlash}/v1`
  }
  return `${defaultSchema}://${value}/v1`
}

/**
 * A page on the public website, which lives on the bare domain — rumi.coach in production,
 * qa.rumi.coach in QA.
 */
export const websiteUrl = (path: string): string => {
  const customUrl = process.env.EXPO_PUBLIC_WEBSITE_URL || process.env.EXPO_PUBLIC_FRONTEND_URL
  const cleanPath = path.replace(/^\/+/, '')
  if (customUrl) {
    const base = customUrl.startsWith('http://') || customUrl.startsWith('https://')
      ? customUrl
      : `https://${customUrl}`
    return `${base.replace(/\/+$/, '')}/${cleanPath}`
  }
  return `${defaultSchema}://${apiDomain}/${cleanPath}`
}

export const authBackendUrl = resolveEndpointUrl(
  process.env.EXPO_PUBLIC_API_AUTH ||
    process.env.EXPO_PUBLIC_AUTH_BACKEND_URL ||
    process.env.EXPO_PUBLIC_API_HOST,
  `auth.${apiDomain}`
)

export function regionBackendUrl(region: string): string {
  const envOverride =
    process.env[`EXPO_PUBLIC_API_${region.toUpperCase()}`] ||
    process.env[`EXPO_PUBLIC_${region.toUpperCase()}_BACKEND_URL`] ||
    process.env.EXPO_PUBLIC_API_HOST

  return resolveEndpointUrl(envOverride, `${region}.${apiDomain}`)
}

export function regionWebSocketUrl(region: string): string {
  const envKey =
    process.env[`EXPO_PUBLIC_WS_${region.toUpperCase()}`] ||
    process.env[`EXPO_PUBLIC_${region.toUpperCase()}_WS_URL`]
  if (envKey) return envKey

  const backend = regionBackendUrl(region)
  const isHttps = backend.startsWith('https://')
  const protocol = isHttps ? 'wss' : 'ws'
  const rest = backend.replace(/^https?:\/\//, '')
  return `${protocol}://${rest}/ws/chat`
}
