import { describe, expect, it, jest } from '@jest/globals'
import { getStorageAdapter } from '../storage'

const mockStore: Record<string, string | null> = {}

jest.mock('expo-secure-store', () => ({
  getItemAsync: jest.fn(async (key: string) => mockStore[key] ?? null),
  setItemAsync: jest.fn(async (key: string, value: string) => {
    mockStore[key] = value
  }),
  deleteItemAsync: jest.fn(async (key: string) => {
    delete mockStore[key]
  }),
}))

describe('storage adapter', () => {
  it('returns a non-null adapter', () => {
    const adapter = getStorageAdapter()
    expect(adapter).toBeDefined()
    expect(adapter.getItemAsync).toBeDefined()
    expect(adapter.setItemAsync).toBeDefined()
    expect(adapter.deleteItemAsync).toBeDefined()
  })

  it('can set, get, and delete an item', async () => {
    const adapter = getStorageAdapter()
    await adapter.setItemAsync('test_key', 'test_value')
    const value = await adapter.getItemAsync('test_key')
    expect(value).toBe('test_value')
    await adapter.deleteItemAsync('test_key')
    const afterDelete = await adapter.getItemAsync('test_key')
    expect(afterDelete).toBeNull()
  })
})
