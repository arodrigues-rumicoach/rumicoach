import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import webStorage from '../storage/web'

const mockLocalStorage: Record<string, string | null> = {}

Object.defineProperty(global, 'localStorage', {
  value: {
    getItem: jest.fn((key: string) => mockLocalStorage[key] ?? null),
    setItem: jest.fn((key: string, value: string) => {
      mockLocalStorage[key] = value
    }),
    removeItem: jest.fn((key: string) => {
      delete mockLocalStorage[key]
    }),
  },
  writable: true,
})

describe('web storage adapter', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    Object.keys(mockLocalStorage).forEach((key) => delete mockLocalStorage[key])
  })

  it('stores non-token keys in localStorage', async () => {
    await webStorage.setItemAsync('theme', 'dark')
    expect(global.localStorage.setItem).toHaveBeenCalled()
    const value = await webStorage.getItemAsync('theme')
    expect(value).toBe('dark')
  })

  it('stores auth tokens obfuscated, not in plaintext', async () => {
    await webStorage.setItemAsync('rumi_auth_token', 'secret-token')
    expect(mockLocalStorage['rumi_enc:rumi_auth_token']).toBeDefined()
    expect(mockLocalStorage['rumi_enc:rumi_auth_token']).not.toBe('secret-token')
    const value = await webStorage.getItemAsync('rumi_auth_token')
    expect(value).toBe('secret-token')
  })

  it('round-trips the refresh token', async () => {
    await webStorage.setItemAsync('rumi_refresh_token', 'secret-refresh')
    const value = await webStorage.getItemAsync('rumi_refresh_token')
    expect(value).toBe('secret-refresh')
  })

  it('deletes tokens from localStorage', async () => {
    await webStorage.setItemAsync('rumi_auth_token', 'old-token')
    await webStorage.deleteItemAsync('rumi_auth_token')
    expect(mockLocalStorage['rumi_enc:rumi_auth_token']).toBeUndefined()
    const value = await webStorage.getItemAsync('rumi_auth_token')
    expect(value).toBeNull()
  })
})
