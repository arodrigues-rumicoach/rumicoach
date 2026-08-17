import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { Platform } from 'react-native'
import { render } from '@testing-library/react-native'

import OrganicVisualizer from '../OrganicVisualizer'

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

jest.mock('../../../hooks/useSettings', () => ({
  useSettings: () => ({
    colorScheme: {
      primary: '#ff0000',
      secondary: '#00ff00',
    },
  }),
}))

describe('OrganicVisualizer', () => {
  it('renders without crashing on web', async () => {
    setPlatformOS('web')
    const { toJSON } = await render(<OrganicVisualizer status="disconnected" inputVolume={0} />)
    expect(toJSON()).toBeTruthy()
    expect(toJSON()?.children?.length).toBe(3)
  })

  it('renders without crashing on ios', async () => {
    setPlatformOS('ios')
    const { toJSON } = await render(<OrganicVisualizer status="disconnected" inputVolume={0} />)
    expect(toJSON()).toBeTruthy()
    expect(toJSON()?.children?.length).toBe(3)
  })

  it('renders without crashing on android', async () => {
    setPlatformOS('android')
    const { toJSON } = await render(<OrganicVisualizer status="disconnected" inputVolume={0} />)
    expect(toJSON()).toBeTruthy()
    expect(toJSON()?.children?.length).toBe(3)
  })

  it('renders three animated circles for different statuses', async () => {
    setPlatformOS('ios')
    const statuses = ['disconnected', 'listening', 'thinking', 'speaking'] as const
    for (const status of statuses) {
      const { toJSON } = await render(<OrganicVisualizer status={status} inputVolume={0.5} />)
      expect(toJSON()?.children?.length).toBe(3)
    }
  })
})
