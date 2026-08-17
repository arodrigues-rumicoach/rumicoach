import React from 'react'
import { describe, expect, it, jest, beforeEach } from '@jest/globals'
import { render, fireEvent } from '@testing-library/react-native'
import { TamaguiProvider } from 'tamagui'
import tamaguiConfig from '../../../../tamagui.config'
import { CoachPreferenceStep } from '../auth/CoachPreferenceStep'

jest.mock('react-native-reanimated', () => jest.requireActual('../../../__mocks__/react-native-reanimated'))

jest.mock('../../../hooks/useVoicePreview', () => ({
  useVoicePreview: jest.fn(() => ({
    playingVoice: null,
    loadingVoice: null,
    play: jest.fn(),
    stop: jest.fn(),
  })),
}))

jest.mock('expo-haptics', () => ({
  impactAsync: jest.fn(),
  ImpactFeedbackStyle: { Light: 'light', Medium: 'medium' },
}))

jest.mock('../../../i18n', () => {
  const mockI18n = {
    t: jest.fn((key: string, fallback?: string) => fallback || key),
    locale: 'en-US',
  }
  return { __esModule: true, default: mockI18n }
})

const defaultProps = {
  coachGender: null as 'male' | 'female' | null,
  coachVoice: null as string | null,
  onGenderChange: jest.fn(),
  onVoiceChange: jest.fn(),
  onContinueWithVoice: jest.fn(),
  onFillInManually: jest.fn(),
  loading: false,
}

const renderStep = (props = defaultProps) =>
  render(
    <TamaguiProvider config={tamaguiConfig} defaultTheme="dark">
      <CoachPreferenceStep {...props} />
    </TamaguiProvider>,
  )

describe('CoachPreferenceStep', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('renders heading and gender buttons', () => {
    const { getByText } = renderStep()
    expect(getByText('Choose your coach')).toBeTruthy()
    expect(getByText('Male')).toBeTruthy()
    expect(getByText('Female')).toBeTruthy()
  })

  it('shows prompt to select gender for voices', () => {
    const { getByText } = renderStep()
    expect(getByText('Select a gender to see available voices')).toBeTruthy()
  })

  it('calls onGenderChange and onVoiceChange when male is selected', () => {
    const onGenderChange = jest.fn()
    const onVoiceChange = jest.fn()
    const { getByText } = renderStep({ ...defaultProps, onGenderChange, onVoiceChange })
    fireEvent.press(getByText('Male'))
    expect(onGenderChange).toHaveBeenCalledWith('male')
    expect(onVoiceChange).toHaveBeenCalledWith('algieba')
  })

  it('calls onGenderChange and onVoiceChange when female is selected', () => {
    const onGenderChange = jest.fn()
    const onVoiceChange = jest.fn()
    const { getByText } = renderStep({ ...defaultProps, onGenderChange, onVoiceChange })
    fireEvent.press(getByText('Female'))
    expect(onGenderChange).toHaveBeenCalledWith('female')
    expect(onVoiceChange).toHaveBeenCalledWith('gacrux')
  })

  it('shows male voices when male is selected', () => {
    const { getByText } = renderStep({ ...defaultProps, coachGender: 'male' })
    expect(getByText('Algieba')).toBeTruthy()
    expect(getByText('Enceladus')).toBeTruthy()
    expect(getByText('Charon')).toBeTruthy()
  })

  it('shows female voices when female is selected', () => {
    const { getByText } = renderStep({ ...defaultProps, coachGender: 'female' })
    expect(getByText('Gacrux')).toBeTruthy()
    expect(getByText('Aoede')).toBeTruthy()
    expect(getByText('Vindemiatrix')).toBeTruthy()
  })

  it('calls onVoiceChange when a voice is selected', () => {
    const onVoiceChange = jest.fn()
    const { getByText } = renderStep({
      ...defaultProps,
      coachGender: 'male',
      onVoiceChange,
    })
    fireEvent.press(getByText('Algieba'))
    expect(onVoiceChange).toHaveBeenCalledWith('algieba')
  })

  it('renders Talk to Rumi button', () => {
    const { getByText } = renderStep()
    expect(getByText('Talk to Rumi')).toBeTruthy()
  })

  it('renders Fill in Manually button', () => {
    const { getByText } = renderStep()
    expect(getByText('Fill in Manually')).toBeTruthy()
  })

  it('calls onContinueWithVoice when Talk to Rumi is pressed', () => {
    const onContinueWithVoice = jest.fn()
    const { getByText } = renderStep({
      ...defaultProps,
      coachGender: 'female',
      coachVoice: 'gacrux',
      onContinueWithVoice,
    })
    fireEvent.press(getByText('Talk to Rumi'))
    expect(onContinueWithVoice).toHaveBeenCalled()
  })

  it('calls onFillInManually when Fill in Manually is pressed', () => {
    const onFillInManually = jest.fn()
    const { getByText } = renderStep({ ...defaultProps, onFillInManually })
    fireEvent.press(getByText('Fill in Manually'))
    expect(onFillInManually).toHaveBeenCalled()
  })
})
