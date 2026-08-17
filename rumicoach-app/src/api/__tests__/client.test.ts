import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import axios from 'axios'
import { AppEvents } from '../../utils/AppEvents'
import { api } from '../client'

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

jest.mock('../../utils/AppEvents', () => ({
  AppEvents: {
    emit: jest.fn(),
  },
}))

const { __mockStorage: mockStorage } = require('../../adapters/storage')

const getRequestHandlers = () => (api.interceptors.request as any).handlers
const getResponseHandlers = () => (api.interceptors.response as any).handlers

const runRequestInterceptor = async (config: any) => {
  // The auth-token interceptor is the first registered request interceptor.
  const handlers = getRequestHandlers()
  const authHandler = handlers[0]
  return authHandler.fulfilled(config)
}

const runResponseInterceptor = async (error: any) => {
  // The refresh-token interceptor is the last registered response interceptor.
  const handlers = getResponseHandlers()
  const lastHandler = handlers[handlers.length - 1]
  return lastHandler.rejected(error)
}

describe('api client token interceptor', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('reads the auth token from the storage adapter and attaches it to requests', async () => {
    mockStorage.getItemAsync.mockResolvedValue('mock-auth-token')

    const config = { headers: {} }
    const result = await runRequestInterceptor(config)

    expect(mockStorage.getItemAsync).toHaveBeenCalledWith('rumi_auth_token')
    expect(result.headers.Authorization).toBe('Bearer mock-auth-token')
  })

  it('sets the baseURL to the region from the token', async () => {
    // JWT with { "region": "us" }
    const usToken = 'eyJhbGciOiJub25lIn0.eyJyZWdpb24iOiJ1cyJ9.'
    mockStorage.getItemAsync.mockResolvedValue(usToken)

    const config = { headers: {} }
    const result = await runRequestInterceptor(config)

    expect(result.baseURL).toBe('https://us.rumi.coach/v1')
  })

  it('writes refreshed tokens to the storage adapter on 401 refresh success', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_auth_token') return 'new-access-token'
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })
    jest.spyOn(axios, 'post').mockResolvedValue({
      data: {
        accessToken: 'new-access-token',
        refreshToken: 'new-refresh-token',
      },
    })
    const requestSpy = jest
      .spyOn(api, 'request')
      .mockResolvedValue({ status: 200, data: {} } as any)

    const error = {
      response: { status: 401 },
      config: { headers: {}, url: '/test', method: 'get' },
    }

    await runResponseInterceptor(error)

    expect(axios.post).toHaveBeenCalledTimes(1)
    expect(axios.post).toHaveBeenCalledWith(
      expect.stringContaining('/auth/refresh'),
      { refreshToken: 'mock-refresh-token' },
      { timeout: 15000 }
    )
    expect(mockStorage.setItemAsync).toHaveBeenCalledWith('rumi_auth_token', 'new-access-token')
    expect(mockStorage.setItemAsync).toHaveBeenCalledWith('rumi_refresh_token', 'new-refresh-token')
    expect(requestSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer new-access-token' }),
      })
    )

    requestSpy.mockRestore()
  })

  it('does not delete tokens on network errors, keeps them for retry', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })
    jest.spyOn(axios, 'post').mockRejectedValue(new Error('Network Error'))

    const error = {
      response: { status: 401 },
      config: { headers: {}, url: '/test', method: 'get' },
    }

    await expect(runResponseInterceptor(error)).rejects.toBeDefined()

    expect(mockStorage.deleteItemAsync).not.toHaveBeenCalled()
    expect(AppEvents.emit).not.toHaveBeenCalled()
  })

  it('does not delete tokens on server 5xx errors, keeps them for retry', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })
    jest.spyOn(axios, 'post').mockRejectedValue({
      response: { status: 500 },
    })

    const error = {
      response: { status: 401 },
      config: { headers: {}, url: '/test', method: 'get' },
    }

    await expect(runResponseInterceptor(error)).rejects.toBeDefined()

    expect(mockStorage.deleteItemAsync).not.toHaveBeenCalled()
    expect(AppEvents.emit).not.toHaveBeenCalled()
  })

  it('deletes tokens when auth backend rejects refresh (400/401/403)', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })
    jest.spyOn(axios, 'post').mockRejectedValue({
      response: { status: 401 },
    })

    const error = {
      response: { status: 401 },
      config: { headers: {}, url: '/test', method: 'get' },
    }

    await expect(runResponseInterceptor(error)).rejects.toBeDefined()

    expect(mockStorage.deleteItemAsync).toHaveBeenCalledWith('rumi_auth_token')
    expect(mockStorage.deleteItemAsync).toHaveBeenCalledWith('rumi_refresh_token')
    expect(AppEvents.emit).toHaveBeenCalledWith('auth:invalid')
  })

  it('emits auth:invalid when no refresh token is present', async () => {
    mockStorage.getItemAsync.mockResolvedValue(null)

    const error = {
      response: { status: 401 },
      config: { headers: {}, url: '/test', method: 'get' },
    }

    await expect(runResponseInterceptor(error)).rejects.toBeDefined()

    expect(AppEvents.emit).toHaveBeenCalledWith('auth:invalid')
  })

  it('deduplicates concurrent refresh token requests', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_auth_token') return 'new-access-token'
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })

    let resolveRefresh: (value: any) => void
    const refreshDeferred = new Promise((resolve) => {
      resolveRefresh = resolve
    })
    const postSpy = jest.spyOn(axios, 'post').mockImplementation(() => refreshDeferred as any)

    const requestSpy = jest
      .spyOn(api, 'request')
      .mockResolvedValue({ status: 200, data: {} } as any)

    const error1 = {
      response: { status: 401 },
      config: { headers: {}, url: '/test1', method: 'get' },
    }
    const error2 = {
      response: { status: 401 },
      config: { headers: {}, url: '/test2', method: 'get' },
    }

    const promise1 = runResponseInterceptor(error1)
    const promise2 = runResponseInterceptor(error2)

    // Yield to let the async interceptor reach the refresh request.
    await new Promise((resolve) => setImmediate(resolve))

    expect(postSpy).toHaveBeenCalledTimes(1)

    resolveRefresh!({
      data: {
        accessToken: 'new-access-token',
        refreshToken: 'new-refresh-token',
      },
    })

    await Promise.all([promise1, promise2])

    expect(postSpy).toHaveBeenCalledTimes(1)
    expect(requestSpy).toHaveBeenCalledTimes(2)

    postSpy.mockRestore()
    requestSpy.mockRestore()
  })

  it('rejects all queued requests when refresh fails with auth error', async () => {
    mockStorage.getItemAsync.mockImplementation(async (key: string) => {
      if (key === 'rumi_refresh_token') return 'mock-refresh-token'
      return null
    })

    let rejectRefresh: (reason?: any) => void
    const refreshDeferred = new Promise((_resolve, reject) => {
      rejectRefresh = reject
    })
    const postSpy = jest.spyOn(axios, 'post').mockImplementation(() => refreshDeferred as any)

    const error1 = {
      response: { status: 401 },
      config: { headers: {}, url: '/test1', method: 'get' },
    }
    const error2 = {
      response: { status: 401 },
      config: { headers: {}, url: '/test2', method: 'get' },
    }

    const promise1 = runResponseInterceptor(error1)
    const promise2 = runResponseInterceptor(error2)

    // Yield to let the async interceptor reach the refresh request.
    await new Promise((resolve) => setImmediate(resolve))

    expect(postSpy).toHaveBeenCalledTimes(1)

    rejectRefresh!({ response: { status: 401 } })

    await expect(promise1).rejects.toBeDefined()
    await expect(promise2).rejects.toBeDefined()

    expect(postSpy).toHaveBeenCalledTimes(1)
    expect(AppEvents.emit).toHaveBeenCalledTimes(1)

    postSpy.mockRestore()
  })
})
