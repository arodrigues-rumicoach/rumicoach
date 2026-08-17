import { Platform } from 'react-native'

export const isWeb = Platform.OS === 'web'
export const isIOS = Platform.OS === 'ios'
export const isAndroid = Platform.OS === 'android'
export const isNative = !isWeb

type NumericInputProps = {
  keyboardType?: 'number-pad' | 'numeric'
  inputMode?: 'numeric' | 'text' | 'none'
}

/**
 * Returns the prop set for a numeric TextInput across platforms.
 * - iOS: no keyboardType. Using number-pad breaks the system "From Messages"
 *   OTP autofill bubble in many iOS versions, so we leave the keyboard type
 *   to the system (textContentType='oneTimeCode' already hints OTP).
 * - Android: number-pad + inputMode="numeric" for autofill hints
 * - Web: inputMode="numeric" (browser hint, no virtual keyboard)
 */
export function numberInputProps(): NumericInputProps {
  if (isWeb) {
    return { inputMode: 'numeric' }
  }
  if (isAndroid) {
    return { keyboardType: 'number-pad', inputMode: 'numeric' }
  }
  return {}
}

type OtpAutofillProps = {
  textContentType?: 'oneTimeCode'
  autoComplete?: 'one-time-code' | 'sms-otp'
  importantForAutofill?: 'yes' | 'no'
  inputMode?: 'numeric'
}

/**
 * Platform-correct props for SMS OTP autofill on a TextInput.
 * - iOS: textContentType="oneTimeCode" + autoComplete="one-time-code"
 * - Android: autoComplete="sms-otp" + importantForAutofill
 * - Web: autoComplete="one-time-code" + inputMode="numeric"
 */
export function otpAutofillProps(): OtpAutofillProps {
  if (isIOS) {
    // Keep only textContentType on iOS. Adding autoComplete here can break the
    // system "From Messages" OTP autofill bubble in some iOS/RN combinations.
    return { textContentType: 'oneTimeCode' }
  }
  if (isAndroid) {
    return { autoComplete: 'sms-otp', importantForAutofill: 'yes' }
  }
  return { autoComplete: 'one-time-code', inputMode: 'numeric' }
}

type GlowShadow = {
  boxShadow?: string
  [key: string]: unknown
}

/**
 * Cross-platform glow / shadow style.
 * Uses boxShadow (supported on all platforms in RN 0.76+).
 */
function withOpacity(color: string, opacity: number): string {
  if (color.startsWith('#')) {
    const hex = color.replace('#', '')
    const r = parseInt(hex.substring(0, 2), 16)
    const g = parseInt(hex.substring(2, 4), 16)
    const b = parseInt(hex.substring(4, 6), 16)
    return `rgba(${r},${g},${b},${opacity})`
  }
  if (color.startsWith('rgba(')) return color
  if (color.startsWith('rgb(')) {
    return color.replace('rgb(', 'rgba(').replace(')', `,${opacity})`)
  }
  return color
}

export function glowShadow(color: string, radius: number, opacity: number): GlowShadow {
  return {
    boxShadow: `0px 0px ${radius}px ${withOpacity(color, opacity)}`,
  }
}

type StackAnimation = {
  animation: 'fade' | 'slide_from_right' | 'slide_from_left' | 'none'
  animationDuration: number
}

/**
 * Stack screen animation preference for expo-router Stack screenOptions.
 * - Native: slide_from_right 280ms
 * - Web: fade 180ms (slide feels janky on web)
 */
export function stackAnimation(): StackAnimation {
  if (isWeb) {
    return { animation: 'fade', animationDuration: 180 }
  }
  return { animation: 'slide_from_right', animationDuration: 280 }
}
