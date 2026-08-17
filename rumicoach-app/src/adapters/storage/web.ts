import type { StorageAdapter } from './types'

const STORAGE_PREFIX = 'rumi_enc:'

function getObfuscationKey(): string {
  const raw =
    typeof navigator !== 'undefined' && typeof location !== 'undefined'
      ? navigator.userAgent + location.origin
      : 'rumi'
  let hash = 0
  for (let i = 0; i < raw.length; i++) {
    const char = raw.charCodeAt(i)
    hash = (hash << 5) - hash + char
    hash |= 0
  }
  return Math.abs(hash).toString(16).padStart(8, '0')
}

function xorObfuscate(input: string): string {
  const key = getObfuscationKey()
  let output = ''
  for (let i = 0; i < input.length; i++) {
    output += String.fromCharCode(input.charCodeAt(i) ^ key.charCodeAt(i % key.length))
  }
  return btoa(output)
}

function xorDeobfuscate(input: string): string | null {
  try {
    const decoded = atob(input)
    const key = getObfuscationKey()
    let output = ''
    for (let i = 0; i < decoded.length; i++) {
      output += String.fromCharCode(decoded.charCodeAt(i) ^ key.charCodeAt(i % key.length))
    }
    return output
  } catch {
    return null
  }
}

const webStorage: StorageAdapter = {
  getItemAsync: async (key) => {
    if (typeof localStorage === 'undefined') return null
    const raw = localStorage.getItem(STORAGE_PREFIX + key)
    if (!raw) return null
    return xorDeobfuscate(raw)
  },
  setItemAsync: async (key, value) => {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(STORAGE_PREFIX + key, xorObfuscate(value))
  },
  deleteItemAsync: async (key) => {
    if (typeof localStorage === 'undefined') return
    localStorage.removeItem(STORAGE_PREFIX + key)
  },
}

export default webStorage
