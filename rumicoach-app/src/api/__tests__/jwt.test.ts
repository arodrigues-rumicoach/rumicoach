import { describe, expect, it } from '@jest/globals'
import { decodeJwt, getRegionFromToken } from '../jwt'

function encodeBase64Url(input: string): string {
  return Buffer.from(input).toString('base64url')
}

function makeToken(payload: Record<string, unknown>): string {
  const header = encodeBase64Url(JSON.stringify({ alg: 'none', typ: 'JWT' }))
  const body = encodeBase64Url(JSON.stringify(payload))
  return `${header}.${body}.`
}

describe('jwt utilities', () => {
  it('decodes a JWT payload', () => {
    const token = makeToken({ region: 'us', sub: 'user-123' })
    const payload = decodeJwt(token)
    expect(payload).toEqual({ region: 'us', sub: 'user-123' })
  })

  it('extracts region from token', () => {
    const token = makeToken({ region: 'us' })
    expect(getRegionFromToken(token)).toBe('us')
  })

  it('defaults to eu when region is missing', () => {
    const token = makeToken({ sub: 'user-123' })
    expect(getRegionFromToken(token)).toBe('eu')
  })

  it('defaults to eu when token is null or invalid', () => {
    expect(getRegionFromToken(null)).toBe('eu')
    expect(getRegionFromToken(undefined)).toBe('eu')
    expect(getRegionFromToken('not-a-jwt')).toBe('eu')
  })
})
