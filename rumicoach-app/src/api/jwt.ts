/**
 * Minimal JWT decoder. We only need to read the payload claim that tells us
 * which regional backend to route authenticated requests to.
 *
 * No signature verification is performed here; the backend is responsible for
 * validating the token.
 */

export interface JwtPayload {
  region?: string
  is_admin?: boolean
  [key: string]: unknown
}

function base64UrlDecode(input: string): string {
  const base64 = input.replace(/-/g, '+').replace(/_/g, '/')
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), '=')
  return typeof Buffer !== 'undefined'
    ? Buffer.from(padded, 'base64').toString('utf8')
    : atob(padded)
}

export function decodeJwt(token: string): JwtPayload | null {
  const parts = token.split('.')
  if (parts.length !== 3) return null

  try {
    const payload = base64UrlDecode(parts[1])
    return JSON.parse(payload) as JwtPayload
  } catch {
    return null
  }
}

export function getRegionFromToken(token: string | null | undefined): string {
  if (!token) return 'eu'
  const payload = decodeJwt(token)
  return payload?.region || 'eu'
}
