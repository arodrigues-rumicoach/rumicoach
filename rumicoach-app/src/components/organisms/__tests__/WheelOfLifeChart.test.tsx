// @ts-nocheck
import React from 'react'
import { describe, expect, it, jest } from '@jest/globals'
import { Platform } from 'react-native'
import { render } from '@testing-library/react-native'

import WheelOfLifeChart from '../WheelOfLifeChartImpl'

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

jest.mock('../../../hooks/useSettings', () => ({
  useSettings: () => ({
    colorScheme: {
      primary: '#1a5f4f',
      secondary: '#d4f0ea',
      tertiary: '#0d3d32',
      accent: '#2d8a6e',
      navIconBlur: '#1a5f4f80',
    },
  }),
}))

jest.mock('react-native-reanimated', () => {
  const Noop = () => null
  const NoopValue = (v) => ({ current: v ?? 0, value: v ?? 0 })
  return {
    __esModule: true,
    default: {
      useSharedValue: NoopValue,
      useDerivedValue: (fn) => ({ value: fn(), addListener: Noop, removeListener: Noop, modify: Noop }),
      useReducedMotion: () => false,
      withTiming: (v) => v,
      withSpring: (v) => v,
      Easing: { linear: (v) => v, ease: (v) => v, out: (v) => v, inOut: (v) => v, bezier: () => (v) => v },
      interpolate: (v) => v,
      runOnJS: (fn) => fn,
      runOnUI: (fn) => fn,
    },
    useSharedValue: NoopValue,
    useDerivedValue: (fn) => ({ value: fn(), addListener: Noop, removeListener: Noop, modify: Noop }),
    useReducedMotion: () => false,
    withTiming: (v) => v,
    withSpring: (v) => v,
    Easing: { linear: (v) => v, ease: (v) => v, out: (v) => v, inOut: (v) => v, bezier: () => (v) => v },
    interpolate: (v) => v,
    runOnJS: (fn) => fn,
    runOnUI: (fn) => fn,
    FadeIn: { duration: () => ({ springify: () => ({}) }) },
    FadeInDown: { duration: () => ({ springify: () => ({}) }) },
  }
})

jest.mock('@shopify/react-native-skia', () => {
  const React = require('react')
  const RN = require('react-native')
  const Stub = (props) => React.createElement(RN.View, props)
  return {
    Canvas: Stub,
    Path: Stub,
    Circle: Stub,
    Text: Stub,
    Group: Stub,
    RadialGradient: () => null,
    DashPathEffect: () => null,
    vec: (x, y) => ({ x: x || 0, y: y || 0 }),
    useFont: () => null,
    Skia: {
      Path: { Make: () => ({ moveTo: () => {}, lineTo: () => {}, quadTo: () => {}, addArc: () => {}, close: () => {} }) },
      Paint: () => ({}),
      XYWHRect: () => ({}),
    },
  }
})

const mockData = [
  { name: 'Health', currentScore: 7, targetScore: 9 },
  { name: 'Career', currentScore: 5, targetScore: 8 },
  { name: 'Finances', currentScore: 6, targetScore: 7 },
  { name: 'Relationships', currentScore: 8, targetScore: 9 },
  { name: 'Fun', currentScore: 4, targetScore: 6 },
  { name: 'Growth', currentScore: 6, targetScore: 8 },
  { name: 'Environment', currentScore: 7, targetScore: 8 },
  { name: 'Spirituality', currentScore: 5, targetScore: 7 },
]

const mockDataNoTarget = [
  { name: 'Health', currentScore: 7 },
  { name: 'Career', currentScore: 5 },
  { name: 'Finances', currentScore: 6 },
  { name: 'Relationships', currentScore: 8 },
]

describe('WheelOfLifeChart', () => {
  it('renders nothing with empty data', async () => {
    setPlatformOS('ios')
    const { toJSON } = await render(<WheelOfLifeChart data={[]} />)
    expect(toJSON()).toBeNull()
  })

  it('renders with data on ios', async () => {
    setPlatformOS('ios')
    const { toJSON } = await render(<WheelOfLifeChart data={mockData} />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders with data on android', async () => {
    setPlatformOS('android')
    const { toJSON } = await render(<WheelOfLifeChart data={mockData} />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders without targets', async () => {
    setPlatformOS('ios')
    const { toJSON } = await render(<WheelOfLifeChart data={mockDataNoTarget} />)
    expect(toJSON()).toBeTruthy()
  })

  it('renders with different category counts', async () => {
    setPlatformOS('ios')
    const smallData = [mockData[0], mockData[1], mockData[2]]
    const { toJSON } = await render(<WheelOfLifeChart data={smallData} />)
    expect(toJSON()).toBeTruthy()
  })
})
