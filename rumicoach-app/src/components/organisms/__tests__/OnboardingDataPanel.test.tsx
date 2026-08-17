import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { render } from '@testing-library/react-native'
import { TamaguiProvider } from 'tamagui'
import tamaguiConfig from '../../../../tamagui.config'
import { OnboardingDataPanel } from '../OnboardingDataPanel'

jest.mock('react-native-reanimated', () => jest.requireActual('../../../__mocks__/react-native-reanimated'))

jest.mock('../../../i18n', () => {
  const mockI18n = {
    t: jest.fn((key: string, fallback?: string) => fallback || key),
    locale: 'en-US',
  }
  return { __esModule: true, default: mockI18n }
})

const renderPanel = (data: Record<string, string>) =>
  render(
    <TamaguiProvider config={tamaguiConfig} defaultTheme="dark">
      <OnboardingDataPanel data={data} />
    </TamaguiProvider>,
  )

describe('OnboardingDataPanel', () => {
  it('shows the full country name for an ISO code', async () => {
    const { getByText, queryByText } = await renderPanel({ country: 'PT' })
    expect(getByText('Portugal')).toBeTruthy()
    expect(queryByText('PT')).toBeNull()
  })

  it('falls back to the raw value for an unknown country', async () => {
    const { getByText } = await renderPanel({ country: 'ZZ' })
    expect(getByText('ZZ')).toBeTruthy()
  })

  it('renders nothing when there is no data', async () => {
    const { toJSON } = await renderPanel({})
    expect(toJSON()).toBeNull()
  })
})
