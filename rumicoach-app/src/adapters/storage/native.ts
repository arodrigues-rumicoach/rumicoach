import * as SecureStore from 'expo-secure-store'
import type { StorageAdapter } from './types'

const nativeStorage: StorageAdapter = {
  getItemAsync: (key) => SecureStore.getItemAsync(key),
  setItemAsync: (key, value) => SecureStore.setItemAsync(key, value),
  deleteItemAsync: (key) => SecureStore.deleteItemAsync(key),
}

export default nativeStorage
