import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { StyleSheet } from 'react-native'
import { render, fireEvent } from '@testing-library/react-native'

import { TamaguiProvider } from 'tamagui'
import tamaguiConfig from '../../../../tamagui.config'
import { BottomNav } from '../BottomNav'

jest.mock('../../../hooks/useSettings', () => ({
  useSettings: () => ({
    colorScheme: {
      primary: '#1a5f4f',
      secondary: '#d4f0ea',
      tertiary: '#0d3d32',
      accent: '#2d8a6e',
      navIcon: '#e0e0e0',
      navIconText: '#ffffff',
      navIconBlur: '#1a5f4f80',
      navBlur: '#1a5f4f60',
    },
  }),
}))

jest.mock('../../../context/ScrollNavContext', () => ({
  useScrollNav: () => ({ isShrunk: false, reset: jest.fn() }),
}))

jest.mock('../../../utils/tabHistory', () => ({
  pushTab: jest.fn(),
}))

jest.mock('react-native-safe-area-context', () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}))

// requireActual: a plain require() of a file inside __mocks__/ is itself
// intercepted by jest's module mock for 'react-native-reanimated' and recurses.
jest.mock('react-native-reanimated', () => jest.requireActual('../../../__mocks__/react-native-reanimated'))

const makeProps = (index = 0) =>
  ({
    state: {
      index,
      routes: [
        { key: 'journey-1', name: 'journey' },
        { key: 'session-1', name: 'session' },
        { key: 'memories-1', name: 'memories' },
        { key: 'profile-1', name: 'profile' },
      ],
    },
    navigation: { navigate: jest.fn() },
    descriptors: {},
    insets: { top: 0, bottom: 0, left: 0, right: 0 },
  }) as any

const renderNav = (props: any) =>
  render(
    <TamaguiProvider config={tamaguiConfig} defaultTheme="dark">
      <BottomNav {...props} />
    </TamaguiProvider>,
  )

describe('BottomNav accessibility', () => {
  it('exposes every tab with role, label, and selected state', async () => {
    const { getAllByRole } = await renderNav(makeProps(0))
    const tabs = getAllByRole('tab')
    expect(tabs).toHaveLength(4)

    const labels = tabs.map((t) => t.props.accessibilityLabel)
    expect(labels).toEqual(['Journey', 'Talk', 'Memories', 'Profile'])

    expect(tabs[0].props.accessibilityState).toEqual({ selected: true })
    expect(tabs[1].props.accessibilityState).toEqual({ selected: false })
  })

  it('moves the selected state with the active route', async () => {
    const { getAllByRole } = await renderNav(makeProps(3))
    const tabs = getAllByRole('tab')
    expect(tabs[3].props.accessibilityState).toEqual({ selected: true })
    expect(tabs[0].props.accessibilityState).toEqual({ selected: false })
  })

  it('keeps tab touch targets at least 44pt tall (WCAG 2.5.5 / HIG minimum)', async () => {
    const { getAllByRole } = await renderNav(makeProps(0))
    for (const tab of getAllByRole('tab')) {
      const style = StyleSheet.flatten(tab.props.style)
      expect(style.height).toBeGreaterThanOrEqual(44)
    }
  })

  it('navigates when a tab is pressed', async () => {
    const props = makeProps(0)
    const { getAllByRole } = await renderNav(props)
    fireEvent.press(getAllByRole('tab')[2])
    expect(props.navigation.navigate).toHaveBeenCalledWith('memories')
  })
})
