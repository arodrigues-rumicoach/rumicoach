import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { Platform , View } from 'react-native'
import { render, waitFor, act } from '@testing-library/react-native'

import type { SettingsContextType } from '../SettingsContext'

jest.mock('../../adapters/storage', () => {
  const mockStorage = {
    getItemAsync: jest.fn(),
    setItemAsync: jest.fn(() => Promise.resolve()),
    deleteItemAsync: jest.fn(),
  }
  return {
    getStorageAdapter: jest.fn(() => mockStorage),
    __mockStorage: mockStorage,
  }
})

let mockUpdateUser: jest.Mock

jest.mock('../../hooks/useAuth', () => {
  mockUpdateUser = jest.fn()
  return {
    useAuth: jest.fn(() => ({ user: null, updateUser: mockUpdateUser })),
    mockUpdateUser,
  }
})

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

describe('SettingsProvider', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders on web using the storage adapter', async () => {
    setPlatformOS('web')
    const { SettingsProvider } = require('../SettingsContext')
    const { getStorageAdapter, __mockStorage } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <View testID="child" />
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
    await waitFor(() => expect(__mockStorage.getItemAsync).toHaveBeenCalledTimes(3))
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(1, 'rumi_visualizer_type')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(2, 'rumi_theme')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(3, 'rumi_shake_to_report')
  })

  it('renders on ios using the storage adapter', async () => {
    setPlatformOS('ios')
    const { SettingsProvider } = require('../SettingsContext')
    const { getStorageAdapter, __mockStorage } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <View testID="child" />
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
    await waitFor(() => expect(__mockStorage.getItemAsync).toHaveBeenCalledTimes(3))
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(1, 'rumi_visualizer_type')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(2, 'rumi_theme')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(3, 'rumi_shake_to_report')
  })

  it('renders on android using the storage adapter', async () => {
    setPlatformOS('android')
    const { SettingsProvider } = require('../SettingsContext')
    const { getStorageAdapter, __mockStorage } = require('../../adapters/storage')

    const { getByTestId } = await render(
      <SettingsProvider>
        <View testID="child" />
      </SettingsProvider>,
    )

    expect(getByTestId('child')).toBeTruthy()
    expect(getStorageAdapter).toHaveBeenCalled()
    await waitFor(() => expect(__mockStorage.getItemAsync).toHaveBeenCalledTimes(3))
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(1, 'rumi_visualizer_type')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(2, 'rumi_theme')
    expect(__mockStorage.getItemAsync).toHaveBeenNthCalledWith(3, 'rumi_shake_to_report')
  })

  it('seeds theme from user.theme on a fresh device with no stored preference', async () => {
    setPlatformOS('ios')

    const { useAuth } = require('../../hooks/useAuth')
    const { __mockStorage } = require('../../adapters/storage')

    useAuth.mockReturnValue({
      user: { theme: 'mountain_lake' },
      updateUser: mockUpdateUser,
    })

    __mockStorage.getItemAsync.mockResolvedValueOnce(null)   // visualizer
    __mockStorage.getItemAsync.mockResolvedValueOnce(null)   // theme (fresh)

    const { SettingsContext, SettingsProvider } = require('../SettingsContext')

    let theme: string | undefined
    let isLoading = true
    function Consumer() {
      const ctx = React.useContext(SettingsContext) as SettingsContextType | undefined
      theme = ctx?.theme
      isLoading = ctx?.isLoading ?? true
      return null
    }

    render(
      <SettingsProvider>
        <Consumer />
      </SettingsProvider>,
    )

    await waitFor(() => expect(isLoading).toBe(false))
    expect(theme).toBe('mountain_lake')
    expect(__mockStorage.setItemAsync).toHaveBeenCalledWith('rumi_theme', 'mountain_lake')
  })

  it('keeps device theme sticky — user.theme does not override an existing stored theme', async () => {
    setPlatformOS('ios')

    const { useAuth } = require('../../hooks/useAuth')
    const { __mockStorage } = require('../../adapters/storage')

    useAuth.mockReturnValue({
      user: { theme: 'sunset_beach' },
      updateUser: mockUpdateUser,
    })

    __mockStorage.getItemAsync.mockResolvedValueOnce(null)      // visualizer
    __mockStorage.getItemAsync.mockResolvedValueOnce('mountain_lake') // theme (device)

    const { SettingsContext, SettingsProvider } = require('../SettingsContext')

    let theme: string | undefined
    let isLoading = true
    function Consumer() {
      const ctx = React.useContext(SettingsContext) as SettingsContextType | undefined
      theme = ctx?.theme
      isLoading = ctx?.isLoading ?? true
      return null
    }

    render(
      <SettingsProvider>
        <Consumer />
      </SettingsProvider>,
    )

    await waitFor(() => expect(isLoading).toBe(false))
    // Device already had a theme — user.theme should NOT override it
    expect(theme).toBe('mountain_lake')
    // No persistence call to overwrite the device theme with user's
    expect(__mockStorage.setItemAsync).not.toHaveBeenCalledWith('rumi_theme', 'sunset_beach')
  })

  it('applyRemoteTheme sticks and is not reverted by user.theme sync', async () => {
    setPlatformOS('ios')

    const { useAuth } = require('../../hooks/useAuth')
    const { __mockStorage } = require('../../adapters/storage')

    useAuth.mockReturnValue({
      user: { theme: 'sunset_beach' },
      updateUser: mockUpdateUser,
    })

    __mockStorage.getItemAsync.mockResolvedValueOnce(null)      // visualizer
    __mockStorage.getItemAsync.mockResolvedValueOnce(null)      // theme (fresh device)

    const { SettingsContext, SettingsProvider } = require('../SettingsContext')

    let applyRemoteTheme: SettingsContextType['applyRemoteTheme'] | undefined
    let theme: string | undefined
    let isLoading = true
    function Consumer() {
      const ctx = React.useContext(SettingsContext) as SettingsContextType | undefined
      theme = ctx?.theme
      isLoading = ctx?.isLoading ?? true
      applyRemoteTheme = ctx?.applyRemoteTheme
      return null
    }

    render(
      <SettingsProvider>
        <Consumer />
      </SettingsProvider>,
    )

    await waitFor(() => expect(isLoading).toBe(false))
    // Fresh device seeds from user (sunset_beach)
    expect(theme).toBe('sunset_beach')

    // Apply AI-picked remote theme
    await act(() => {
      applyRemoteTheme!('mountain_lake')
    })

    expect(theme).toBe('mountain_lake')
    // user.theme was NOT updated (remote themes don't touch the user profile)
    expect(mockUpdateUser).not.toHaveBeenCalled()
  })
})
