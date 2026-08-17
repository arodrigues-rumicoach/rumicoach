import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform , View, Button } from 'react-native'
import { render, waitFor, fireEvent } from '@testing-library/react-native'
import { AudioProvider, AudioContext as RumiAudioContext, type AudioContextType } from '../AudioContext'
import { SettingsProvider } from '../SettingsContext'

let mockIsWeb = false

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
    return mockIsWeb
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
    __mockStorage: mockStorage,
  }
})

jest.mock('../../hooks/useAuth', () => ({
  useAuth: jest.fn(() => ({ user: null, updateUser: jest.fn() })),
}))

jest.mock('../../hooks/useThemeAssetUri', () => ({
  useThemeAssetUri: () => ({ videoUri: 'mock://video.mp4', imageUri: 'mock://image.jpg', audioUri: 'mock://audio.m4a', isLoading: false }),
}))

jest.mock('expo-router', () => ({
  useSegments: jest.fn(() => ['(tabs)', 'journey']),
  useRouter: () => ({ navigate: jest.fn(), replace: jest.fn(), push: jest.fn() }),
}))

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
  mockIsWeb = os === 'web'
}

const VOLUME_KEY = 'rumi_music_volume'

describe('AudioProvider', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    delete (global as any).window
  })

  it('renders on web and attaches a one-time click autoplay guard', async () => {
    setPlatformOS('web')
    const addEventListener = jest.fn()
    const removeEventListener = jest.fn()
    ;(global as any).window = { addEventListener, removeEventListener }
    const { getStorageAdapter } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <AudioProvider>
          <View testID="child" />
        </AudioProvider>
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
    await waitFor(() =>
      expect(addEventListener.mock.calls.filter(([event]) => event === 'click')).toHaveLength(1)
    )
    expect(addEventListener).toHaveBeenCalledWith('click', expect.any(Function), { once: true })
  })

  it('renders on ios using the storage adapter', async () => {
    setPlatformOS('ios')
    const { getStorageAdapter } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <AudioProvider>
          <View testID="child" />
        </AudioProvider>
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
  })

  it('renders on android using the storage adapter', async () => {
    setPlatformOS('android')
    const { getStorageAdapter } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <AudioProvider>
          <View testID="child" />
        </AudioProvider>
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
  })

  it('persists volume changes through the storage adapter', async () => {
    setPlatformOS('ios')
    const { __mockStorage } = require('../../adapters/storage')

    const VolumeButton = () => {
      const { setVolume } = React.useContext(RumiAudioContext as React.Context<AudioContextType>)
      return <Button title="Set volume" onPress={() => setVolume(0.75)} testID="set-volume" />
    }

    const { getByTestId } = await render(
      <SettingsProvider>
        <AudioProvider>
          <VolumeButton />
        </AudioProvider>
      </SettingsProvider>,
    )

    fireEvent.press(getByTestId('set-volume'))

    await waitFor(() => expect(__mockStorage.setItemAsync).toHaveBeenCalledWith(VOLUME_KEY, '0.75'))
  })

  it('pauses ambient audio while splash is visible and resumes when hidden', async () => {
    setPlatformOS('ios')
    const { createAudioPlayer } = require('expo-audio')

    const SplashController = () => {
      const { setSplashVisible } = React.useContext(RumiAudioContext as React.Context<AudioContextType>)
      return (
        <Button
          title="Hide splash"
          onPress={() => setSplashVisible(false)}
          testID="hide-splash"
        />
      )
    }

    const { getByTestId } = await render(
      <SettingsProvider>
        <AudioProvider>
          <SplashController />
        </AudioProvider>
      </SettingsProvider>,
    )

    await waitFor(() => expect(createAudioPlayer).toHaveBeenCalled())

    const player = createAudioPlayer.mock.results[0].value
    expect(player.play).not.toHaveBeenCalled()

    fireEvent.press(getByTestId('hide-splash'))

    await waitFor(() => expect(player.play).toHaveBeenCalled())
  })
})
