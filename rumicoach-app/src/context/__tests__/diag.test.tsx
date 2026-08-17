import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { Platform, View } from 'react-native'
import { render } from '@testing-library/react-native'
import { AudioProvider } from '../AudioContext'
import { SettingsProvider } from '../SettingsContext'

jest.mock('expo-audio', () => ({
  createAudioPlayer: jest.fn(() => ({
    play: jest.fn(),
    pause: jest.fn(),
    replace: jest.fn(),
    volume: 0,
    addListener: jest.fn(() => ({ remove: jest.fn() })),
  })),
}))

jest.mock('../../utils/theme', () => ({
  THEME_ASSETS: { waterfall: { audio: 1 } },
}))

jest.mock('../../adapters/platform', () => ({
  get isWeb() {
    return true
  },
}))

jest.mock('../../adapters/storage', () => {
  const mockStorage = {
    getItemAsync: jest.fn(() => Promise.resolve(null)),
    setItemAsync: jest.fn(() => Promise.resolve()),
    deleteItemAsync: jest.fn(() => Promise.resolve()),
  }
  return {
    getStorageAdapter: jest.fn(() => mockStorage),
  }
})

jest.mock('../../hooks/useAuth', () => ({
  useAuth: jest.fn(() => ({ user: null, updateUser: jest.fn() })),
}))

jest.mock('../../hooks/useThemeAssetUri', () => ({
  useThemeAssetUri: () => ({ videoUri: 'mock://video.mp4', imageUri: 'mock://image.jpg', audioUri: 'mock://audio.m4a', isLoading: false }),
}))

;(Platform as { OS: string }).OS = 'web'

describe('diag', () => {
  it('counts add/remove listener calls', async () => {
    const addEventListener = jest.fn()
    const removeEventListener = jest.fn()
    ;(global as any).window = { addEventListener, removeEventListener }

    const { unmount } = await render(
      <SettingsProvider>
        <AudioProvider>
          <View testID="child" />
        </AudioProvider>
      </SettingsProvider>,
    )

    console.log('ADD calls:', addEventListener.mock.calls.length)
    console.log('REMOVE calls:', removeEventListener.mock.calls.length)
    unmount()
    console.log('AFTER UNMOUNT REMOVE calls:', removeEventListener.mock.calls.length)
    expect(true).toBe(true)
  })
})
