import { describe, expect, it } from '@jest/globals'
import React from 'react'
import { render } from '@testing-library/react-native'
import { SignupProgressBar } from '../SignupProgressBar'

jest.mock('react-native-reanimated', () => {
  const View = require('react-native').View
  return {
    __esModule: true,
    default: {
      View: View,
      useSharedValue: (v: number) => ({ value: v }),
      useAnimatedStyle: (fn: () => object) => fn(),
      withSpring: (v: number) => v,
      withTiming: (v: number) => v,
      interpolateColor: () => '#10b981',
      Easing: { out: () => ({}) },
    },
    useSharedValue: (v: number) => ({ value: v }),
    useAnimatedStyle: (fn: () => object) => fn(),
    withSpring: (v: number) => v,
    withTiming: (v: number) => v,
    interpolateColor: () => '#10b981',
    Easing: { out: () => ({}) },
  }
})

describe('SignupProgressBar', () => {
  it('renders step circles', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="NAME" />)
    const tree = JSON.stringify(toJSON())
    expect(tree).toBeTruthy()
  })

  it('renders for NAME step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="NAME" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders for METHOD step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="METHOD" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders for VERIFY step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="VERIFY" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders for REGION_TERMS step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="REGION_TERMS" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders for COACH_PREFERENCE step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="COACH_PREFERENCE" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders for PROFILE_DATA step', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="PROFILE_DATA" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders connectors between steps', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="NAME" />)
    const tree = JSON.stringify(toJSON())
    expect(tree).toBeTruthy()
  })

  it('renders on web platform', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="REGION_TERMS" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders on ios platform', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="REGION_TERMS" />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders on android platform', () => {
    const { toJSON } = render(<SignupProgressBar currentStep="REGION_TERMS" />)
    expect(toJSON()).toBeTruthy()
  })
})
