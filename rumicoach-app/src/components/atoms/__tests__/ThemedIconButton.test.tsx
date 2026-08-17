import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { Text } from 'react-native'
import { render } from '@testing-library/react-native'

import { ThemedIconButton } from '../ThemedIconButton'

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

describe('ThemedIconButton accessibility', () => {
  it('exposes the button role and passes accessibility labels through', async () => {
    const { getByRole } = await render(
      <ThemedIconButton accessibilityLabel="Open settings" onPress={() => {}}>
        <Text>x</Text>
      </ThemedIconButton>,
    )
    const button = getByRole('button')
    expect(button.props.accessibilityLabel).toBe('Open settings')
  })

  // 36pt md button + 4pt hitSlop per side = 44pt; 28pt sm + 8pt = 44pt.
  it.each([
    { size: 'sm' as const, box: 28, slop: 8 },
    { size: 'md' as const, box: 36, slop: 4 },
    { size: 'lg' as const, box: 44, slop: 4 },
  ])('keeps the $size touch target at 44pt or more', async ({ size, box, slop }) => {
    const { getByRole } = await render(
      <ThemedIconButton size={size} accessibilityLabel="btn" onPress={() => {}}>
        <Text>x</Text>
      </ThemedIconButton>,
    )
    expect(getByRole('button').props.hitSlop).toBe(slop)
    expect(box + 2 * slop).toBeGreaterThanOrEqual(44)
  })
})
