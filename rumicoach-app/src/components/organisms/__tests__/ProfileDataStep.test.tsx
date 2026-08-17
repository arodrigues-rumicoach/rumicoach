import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { render, fireEvent } from '@testing-library/react-native'
import { TamaguiProvider } from 'tamagui'
import tamaguiConfig from '../../../../tamagui.config'
import { ProfileDataStep } from '../auth/ProfileDataStep'

jest.mock('react-native-reanimated', () => jest.requireActual('../../../__mocks__/react-native-reanimated'))

jest.mock('../../../i18n', () => {
  const translations: Record<string, string> = {
    manual_entry_title: 'Your Details',
    date_of_birth: 'Date of Birth',
    select_date_of_birth: 'Select Date of Birth',
    gender: 'Gender',
    male: 'Male',
    female: 'Female',
    other: 'Other',
    country: 'Country',
    select_country: 'Select Country',
  }
  const mockI18n = {
    t: jest.fn((key: string) => translations[key] || key),
    locale: 'en-US',
  }
  return { __esModule: true, default: mockI18n, useI18n: () => mockI18n }
})

const defaultProps = {
  dateOfBirth: '',
  gender: '',
  country: '',
  onDateOfBirthChange: jest.fn(),
  onGenderChange: jest.fn(),
  onCountryChange: jest.fn(),
}

const renderStep = (props = defaultProps) =>
  render(
    <TamaguiProvider config={tamaguiConfig} defaultTheme="dark">
      <ProfileDataStep {...props} />
    </TamaguiProvider>,
  )

describe('ProfileDataStep', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders the heading', async () => {
    const { getByText } = await renderStep()
    expect(getByText('Your Details')).toBeTruthy()
  })

  it('renders date of birth field', async () => {
    const { getByText } = await renderStep()
    expect(getByText('Date of Birth')).toBeTruthy()
  })

  it('renders gender selection buttons', async () => {
    const { getByText } = await renderStep()
    expect(getByText('Male')).toBeTruthy()
    expect(getByText('Female')).toBeTruthy()
    expect(getByText('Other')).toBeTruthy()
  })

  it('calls onGenderChange when male is selected', async () => {
    const onGenderChange = jest.fn()
    const { getByText } = await renderStep({ ...defaultProps, onGenderChange })
    fireEvent.press(getByText('Male'))
    expect(onGenderChange).toHaveBeenCalledWith('male')
  })

  it('calls onGenderChange when female is selected', async () => {
    const onGenderChange = jest.fn()
    const { getByText } = await renderStep({ ...defaultProps, onGenderChange })
    fireEvent.press(getByText('Female'))
    expect(onGenderChange).toHaveBeenCalledWith('female')
  })

  it('calls onGenderChange when other is selected', async () => {
    const onGenderChange = jest.fn()
    const { getByText } = await renderStep({ ...defaultProps, onGenderChange })
    fireEvent.press(getByText('Other'))
    expect(onGenderChange).toHaveBeenCalledWith('other')
  })

  it('renders country field', async () => {
    const { getByText } = await renderStep()
    expect(getByText('Country')).toBeTruthy()
  })
})
