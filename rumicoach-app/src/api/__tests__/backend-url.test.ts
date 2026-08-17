import { describe, expect, it, jest } from '@jest/globals'

describe('backend url resolution', () => {
  const originalEnv = process.env

  beforeEach(() => {
    jest.resetModules()
    process.env = { ...originalEnv }
    delete process.env.EXPO_PUBLIC_AUTH_BACKEND_URL
    delete process.env.EXPO_PUBLIC_EU_BACKEND_URL
    delete process.env.EXPO_PUBLIC_US_BACKEND_URL
    delete process.env.EXPO_PUBLIC_EU_WS_URL
    delete process.env.EXPO_PUBLIC_US_WS_URL
    // Both spellings of the domain, or a value leaking in from the shell would decide
    // these assertions instead of the test.
    delete process.env.EXPO_PUBLIC_API
    delete process.env.EXPO_PUBLIC_API_AUTH
    delete process.env.EXPO_PUBLIC_API_EU
    delete process.env.EXPO_PUBLIC_API_US
    delete process.env.EXPO_PUBLIC_API_HOST
    delete process.env.EXPO_PUBLIC_API_SCHEMA
    delete process.env.EXPO_PUBLIC_API_PORT
    delete process.env.EXPO_PUBLIC_WEBSITE_URL
    delete process.env.EXPO_PUBLIC_FRONTEND_URL
  })

  afterAll(() => {
    process.env = originalEnv
  })

  it('authBackendUrl falls back to auth.rumi.coach', () => {
    jest.isolateModules(() => {
      const { authBackendUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.rumi.coach/v1')
    })
  })

  it('authBackendUrl uses EXPO_PUBLIC_AUTH_BACKEND_URL env var', () => {
    process.env.EXPO_PUBLIC_AUTH_BACKEND_URL = 'https://auth.test/v1'
    jest.isolateModules(() => {
      const { authBackendUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.test/v1')
    })
  })

  it('regionBackendUrl falls back to {region}.rumi.coach', () => {
    jest.isolateModules(() => {
      const { regionBackendUrl } = require('../backend-url')
      expect(regionBackendUrl('eu')).toBe('https://eu.rumi.coach/v1')
      expect(regionBackendUrl('us')).toBe('https://us.rumi.coach/v1')
    })
  })

  it('regionBackendUrl uses env var for specific region', () => {
    process.env.EXPO_PUBLIC_US_BACKEND_URL = 'https://us.test/v1'
    jest.isolateModules(() => {
      const { regionBackendUrl } = require('../backend-url')
      expect(regionBackendUrl('us')).toBe('https://us.test/v1')
      expect(regionBackendUrl('eu')).toBe('https://eu.rumi.coach/v1')
    })
  })

  it('prefixes every host from EXPO_PUBLIC_API', () => {
    process.env.EXPO_PUBLIC_API = 'qa.rumi.coach'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, regionWebSocketUrl, websiteUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.qa.rumi.coach/v1')
      expect(regionBackendUrl('eu')).toBe('https://eu.qa.rumi.coach/v1')
      expect(regionBackendUrl('us')).toBe('https://us.qa.rumi.coach/v1')
      expect(regionWebSocketUrl('eu')).toBe('wss://eu.qa.rumi.coach/v1/ws/chat')
      expect(websiteUrl('en/support')).toBe('https://qa.rumi.coach/en/support')
    })
  })

  // The pre-EXPO_PUBLIC_API spelling. Kept working so a .env or CI step that still sets it
  // resolves to the same hosts rather than silently falling back to production.
  it('still accepts EXPO_PUBLIC_API_AUTH and strips its auth. prefix', () => {
    process.env.EXPO_PUBLIC_API_AUTH = 'auth.qa.rumi.coach'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, websiteUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.qa.rumi.coach/v1')
      expect(regionBackendUrl('eu')).toBe('https://eu.qa.rumi.coach/v1')
      expect(regionBackendUrl('us')).toBe('https://us.qa.rumi.coach/v1')
      expect(websiteUrl('about')).toBe('https://qa.rumi.coach/about')
    })
  })

  it('EXPO_PUBLIC_API_AUTH overrides auth while other endpoints use EXPO_PUBLIC_API', () => {
    process.env.EXPO_PUBLIC_API = 'qa.rumi.coach'
    process.env.EXPO_PUBLIC_API_AUTH = 'auth.custom.coach'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.custom.coach/v1')
      expect(regionBackendUrl('eu')).toBe('https://eu.qa.rumi.coach/v1')
    })
  })

  it('websiteUrl uses EXPO_PUBLIC_WEBSITE_URL override when set', () => {
    process.env.EXPO_PUBLIC_API = 'localhost'
    process.env.EXPO_PUBLIC_WEBSITE_URL = 'https://qa.rumi.coach'
    jest.isolateModules(() => {
      const { websiteUrl } = require('../backend-url')
      expect(websiteUrl('en/support')).toBe('https://qa.rumi.coach/en/support')
      expect(websiteUrl('/en/about')).toBe('https://qa.rumi.coach/en/about')
    })
  })

  it('websiteUrl uses EXPO_PUBLIC_FRONTEND_URL override when set', () => {
    process.env.EXPO_PUBLIC_FRONTEND_URL = 'https://custom.rumi.coach/'
    jest.isolateModules(() => {
      const { websiteUrl } = require('../backend-url')
      expect(websiteUrl('terms')).toBe('https://custom.rumi.coach/terms')
    })
  })

  it('supports EXPO_PUBLIC_API_AUTH, EXPO_PUBLIC_API_EU, EXPO_PUBLIC_API_US overrides for local development', () => {
    process.env.EXPO_PUBLIC_API = 'qa.rumi.coach'
    process.env.EXPO_PUBLIC_API_AUTH = 'http://localhost:8000'
    process.env.EXPO_PUBLIC_API_EU = 'http://localhost:8000'
    process.env.EXPO_PUBLIC_API_US = 'http://localhost:8000'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, regionWebSocketUrl, websiteUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('http://localhost:8000/v1')
      expect(regionBackendUrl('eu')).toBe('http://localhost:8000/v1')
      expect(regionBackendUrl('us')).toBe('http://localhost:8000/v1')
      expect(regionWebSocketUrl('eu')).toBe('ws://localhost:8000/v1/ws/chat')
      expect(websiteUrl('en/support')).toBe('https://qa.rumi.coach/en/support')
    })
  })

  it('strips https prefix from EXPO_PUBLIC_API if provided with protocol', () => {
    process.env.EXPO_PUBLIC_API = 'https://qa.rumi.coach'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, websiteUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('https://auth.qa.rumi.coach/v1')
      expect(regionBackendUrl('eu')).toBe('https://eu.qa.rumi.coach/v1')
      expect(websiteUrl('about')).toBe('https://qa.rumi.coach/about')
    })
  })

  it('supports full URLs in EXPO_PUBLIC_API_AUTH and EXPO_PUBLIC_API_EU', () => {
    process.env.EXPO_PUBLIC_API = 'qa.rumi.coach'
    process.env.EXPO_PUBLIC_API_AUTH = 'http://127.0.0.1:8000/v1'
    process.env.EXPO_PUBLIC_API_EU = 'http://127.0.0.1:8000'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, regionWebSocketUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('http://127.0.0.1:8000/v1')
      expect(regionBackendUrl('eu')).toBe('http://127.0.0.1:8000/v1')
      expect(regionWebSocketUrl('eu')).toBe('ws://127.0.0.1:8000/v1/ws/chat')
    })
  })

  it('regionBackendUrl and authBackendUrl route to EXPO_PUBLIC_API_HOST when set', () => {
    process.env.EXPO_PUBLIC_API_HOST = 'http://api.test.local:8000'
    jest.isolateModules(() => {
      const { authBackendUrl, regionBackendUrl, regionWebSocketUrl } = require('../backend-url')
      expect(authBackendUrl).toBe('http://api.test.local:8000/v1')
      expect(regionBackendUrl('eu')).toBe('http://api.test.local:8000/v1')
      expect(regionBackendUrl('us')).toBe('http://api.test.local:8000/v1')
      expect(regionWebSocketUrl('eu')).toBe('ws://api.test.local:8000/v1/ws/chat')
    })
  })

  it('regionWebSocketUrl derives from regionBackendUrl', () => {
    jest.isolateModules(() => {
      const { regionWebSocketUrl } = require('../backend-url')
      expect(regionWebSocketUrl('eu')).toBe('wss://eu.rumi.coach/v1/ws/chat')
    })
  })

  it('regionWebSocketUrl uses env var for specific region', () => {
    process.env.EXPO_PUBLIC_US_WS_URL = 'wss://ws.us.test/v1/chat'
    jest.isolateModules(() => {
      const { regionWebSocketUrl } = require('../backend-url')
      expect(regionWebSocketUrl('us')).toBe('wss://ws.us.test/v1/chat')
    })
  })
})
