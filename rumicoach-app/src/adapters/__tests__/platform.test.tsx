import { describe, expect, it, jest } from '@jest/globals'
import { Platform } from 'react-native'

const setPlatformOS = (os: 'web' | 'ios' | 'android') => {
  ;(Platform as { OS: string }).OS = os
}

const importFreshPlatform = () => {
  let mod: typeof import('../platform') | undefined
  jest.isolateModules(() => {
    mod = require('../platform')
  })
  return mod!
}

describe('platform adapter', () => {
  it('isWeb is true only on web', () => {
    jest.resetModules()
    setPlatformOS('web')
    const web = require('../platform') as typeof import('../platform')
    expect(web.isWeb).toBe(true)

    jest.resetModules()
    setPlatformOS('ios')
    const ios = require('../platform') as typeof import('../platform')
    expect(ios.isWeb).toBe(false)

    jest.resetModules()
    setPlatformOS('android')
    const android = require('../platform') as typeof import('../platform')
    expect(android.isWeb).toBe(false)
  })

  it('isIOS / isAndroid / isNative are mutually exclusive', () => {
    jest.resetModules()
    setPlatformOS('web')
    const web = require('../platform') as typeof import('../platform')
    expect(web.isWeb).toBe(true)
    expect(web.isNative).toBe(false)
    expect(web.isIOS).toBe(false)
    expect(web.isAndroid).toBe(false)

    jest.resetModules()
    setPlatformOS('ios')
    const ios = require('../platform') as typeof import('../platform')
    expect(ios.isIOS).toBe(true)
    expect(ios.isNative).toBe(true)
    expect(ios.isWeb).toBe(false)
    expect(ios.isAndroid).toBe(false)

    jest.resetModules()
    setPlatformOS('android')
    const android = require('../platform') as typeof import('../platform')
    expect(android.isAndroid).toBe(true)
    expect(android.isNative).toBe(true)
    expect(android.isWeb).toBe(false)
    expect(android.isIOS).toBe(false)
  })

  describe('numberInputProps', () => {
    it('returns no keyboardType for iOS', () => {
      jest.resetModules()
      setPlatformOS('ios')
      const { numberInputProps } = require('../platform') as typeof import('../platform')
      expect(numberInputProps()).toEqual({})
    })

    it('returns number-pad + inputMode for Android', () => {
      jest.resetModules()
      setPlatformOS('android')
      const { numberInputProps } = require('../platform') as typeof import('../platform')
      expect(numberInputProps()).toEqual({
        keyboardType: 'number-pad',
        inputMode: 'numeric',
      })
    })

    it('returns inputMode for web', () => {
      jest.resetModules()
      setPlatformOS('web')
      const { numberInputProps } = require('../platform') as typeof import('../platform')
      expect(numberInputProps()).toEqual({ inputMode: 'numeric' })
    })
  })

  describe('otpAutofillProps', () => {
    it('returns oneTimeCode for iOS', () => {
      jest.resetModules()
      setPlatformOS('ios')
      const { otpAutofillProps } = require('../platform') as typeof import('../platform')
      expect(otpAutofillProps()).toEqual({
        textContentType: 'oneTimeCode',
      })
    })

    it('returns sms-otp for Android', () => {
      jest.resetModules()
      setPlatformOS('android')
      const { otpAutofillProps } = require('../platform') as typeof import('../platform')
      expect(otpAutofillProps()).toEqual({
        autoComplete: 'sms-otp',
        importantForAutofill: 'yes',
      })
    })

    it('returns one-time-code for web', () => {
      jest.resetModules()
      setPlatformOS('web')
      const { otpAutofillProps } = require('../platform') as typeof import('../platform')
      expect(otpAutofillProps()).toEqual({
        autoComplete: 'one-time-code',
        inputMode: 'numeric',
      })
    })
  })

  describe('glowShadow', () => {
    it('returns boxShadow on web', () => {
      jest.resetModules()
      setPlatformOS('web')
      const { glowShadow } = require('../platform') as typeof import('../platform')
      const result = glowShadow('#ff0000', 12, 0.5)
      expect(result.boxShadow).toBe('0px 0px 12px rgba(255,0,0,0.5)')
    })

    it('returns boxShadow on Android', () => {
      jest.resetModules()
      setPlatformOS('android')
      const { glowShadow } = require('../platform') as typeof import('../platform')
      const result = glowShadow('#ff0000', 12, 0.5)
      expect(result.boxShadow).toBe('0px 0px 12px rgba(255,0,0,0.5)')
    })

    it('returns boxShadow on iOS', () => {
      jest.resetModules()
      setPlatformOS('ios')
      const { glowShadow } = require('../platform') as typeof import('../platform')
      const result = glowShadow('#ff0000', 12, 0.5)
      expect(result.boxShadow).toBe('0px 0px 12px rgba(255,0,0,0.5)')
    })
  })

  describe('stackAnimation', () => {
    it('returns slide_from_right on iOS', () => {
      jest.resetModules()
      setPlatformOS('ios')
      const { stackAnimation } = require('../platform') as typeof import('../platform')
      expect(stackAnimation()).toEqual({
        animation: 'slide_from_right',
        animationDuration: 280,
      })
    })

    it('returns slide_from_right on Android', () => {
      jest.resetModules()
      setPlatformOS('android')
      const { stackAnimation } = require('../platform') as typeof import('../platform')
      expect(stackAnimation()).toEqual({
        animation: 'slide_from_right',
        animationDuration: 280,
      })
    })

    it('returns fade on web', () => {
      jest.resetModules()
      setPlatformOS('web')
      const { stackAnimation } = require('../platform') as typeof import('../platform')
      expect(stackAnimation()).toEqual({
        animation: 'fade',
        animationDuration: 180,
      })
    })
  })
})

void importFreshPlatform
