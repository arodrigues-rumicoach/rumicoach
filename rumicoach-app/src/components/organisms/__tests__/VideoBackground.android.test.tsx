import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform, View } from 'react-native'
import { render, waitFor } from '@testing-library/react-native'

function findNodesByType(node: any, type: string): any[] {
  if (!node) return []
  if (node.type === type) return [node]
  const children = node.children || []
  return children.flatMap((child: any) => findNodesByType(child, type))
}

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

jest.mock('../../../hooks/useThemeAssetUri', () => ({
  useThemeAssetUri: () => ({ videoUri: 'mock://video.mp4', imageUri: 'mock://image.jpg', audioUri: 'mock://audio.m4a', isLoading: false }),
}))

jest.mock('react-native/Libraries/AppState/AppState', () => ({
  __esModule: true,
  default: {
    addEventListener: jest.fn(() => ({ remove: jest.fn() })),
  },
}))

;(Platform as { OS: string }).OS = 'android'
const { VideoBackground } = require('../VideoBackground')

describe('VideoBackground on android', () => {
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

  it('falls back to image on low-memory devices', async () => {
    const { toJSON } = await render(
      <VideoBackground>
        <View testID="child" />
      </VideoBackground>
    )
    expect(findNodesByType(toJSON(), 'Image').length).toBeGreaterThan(0)
  })

})
