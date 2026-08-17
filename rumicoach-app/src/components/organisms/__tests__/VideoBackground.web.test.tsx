import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform, View } from 'react-native'
import { render, waitFor } from '@testing-library/react-native'

const mockUseVideoPlayer = jest.fn()
const mockPlayer = {
  loop: false,
  muted: false,
  play: jest.fn(),
  replaceAsync: jest.fn(() => Promise.resolve()),
}

jest.mock('expo-video', () => {
  const React = require('react')
  const { View } = require('react-native')
  return {
    VideoView: ({ player }: { player: typeof mockPlayer }) =>
      React.createElement(View, { testID: 'video-view', 'data-player': player }),
    useVideoPlayer: (...args: any[]) => mockUseVideoPlayer(...args),
  }
})

jest.mock('expo-device', () => ({
  totalMemory: 2 * 1024 * 1024 * 1024,
}))

jest.mock('../../../hooks/useSettings', () => ({
  useSettings: () => ({ theme: 'lavender' }),
}))

jest.mock('../../../hooks/useSession', () => ({
  useSession: () => ({ status: 'disconnected' }),
}))

jest.mock('react-native/Libraries/AppState/AppState', () => ({
  __esModule: true,
  default: {
    addEventListener: jest.fn(() => ({ remove: jest.fn() })),
  },
}))

jest.mock('../../../hooks/useThemeAssetUri', () => ({
  useThemeAssetUri: () => ({ videoUri: 'mock://video.mp4', imageUri: 'mock://image.jpg', audioUri: 'mock://audio.m4a', isLoading: false }),
}))

;(Platform as { OS: string }).OS = 'web'
const { VideoBackground } = require('../VideoBackground')

describe('VideoBackground on web', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockUseVideoPlayer.mockImplementation((source: any, callback: any) => {
      callback(mockPlayer)
      return mockPlayer
    })
  })

  it('renders without crashing', async () => {
    const { getByTestId } = await render(
      <VideoBackground>
        <View testID="child" />
      </VideoBackground>
    )
    expect(getByTestId('child')).toBeTruthy()
  })

  it('renders video regardless of low device memory', async () => {
    const { queryByTestId } = await render(
      <VideoBackground>
        <View testID="child" />
      </VideoBackground>
    )
    expect(queryByTestId('video-view')).toBeTruthy()
  })

  it('loads the resolved video URI into the player', async () => {
    render(
      <VideoBackground>
        <View testID="child" />
      </VideoBackground>
    )
    await waitFor(() => {
      expect(mockPlayer.replaceAsync).toHaveBeenCalledWith('mock://video.mp4')
    })
  })
})
