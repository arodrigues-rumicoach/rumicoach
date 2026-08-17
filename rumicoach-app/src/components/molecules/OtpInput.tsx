import { memo, useCallback, useEffect, useRef } from 'react'
import {
  Pressable,
  TextInput,
  View,
  StyleSheet,
} from 'react-native'
import { Text, XStack } from 'tamagui'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  withSpring,
  withSequence,
  withDelay,
  withRepeat,
  cancelAnimation,
  Easing,
  interpolateColor,
} from 'react-native-reanimated'
import * as Clipboard from 'expo-clipboard'
import { useSettings } from '@/hooks/useSettings'
import { useOtpCascade } from '@/hooks/useOtpCascade'
import { haptic } from '@/utils/haptics'
import {
  numberInputProps,
  otpAutofillProps,
  isWeb,
} from '@/adapters/platform'

interface OtpInputProps {
  value: string
  onChange: (value: string) => void
  length?: number
  onComplete?: (value: string) => void
  error?: boolean
  disabled?: boolean
  autoFocus?: boolean
  /** 'light' = frosted white glass with dark ink, 'dark' = legacy dark glass with white ink */
  variant?: 'light' | 'dark'
}

const CELL_SIZE = 48
const CELL_GAP = 8
const ENTRY_DURATION = 220
const BORDER_PULSE_IN = 160
const BORDER_PULSE_HOLD = 380
const BORDER_PULSE_OUT = 320
const DIGIT_TRANSLATE_Y = 12
const ON_COMPLETE_DELAY_MS = 280
const LIGHT_IDLE_BORDER = 'rgba(0,0,0,0.10)'
const LIGHT_IDLE_BACKGROUND = 'rgba(0,0,0,0.05)'
const DARK_IDLE_BORDER = 'rgba(255,255,255,0.12)'
const DARK_IDLE_BACKGROUND = 'rgba(255,255,255,0.05)'
const ERROR_BORDER = '#ef4444'
const INK_PRIMARY = '#262220'

interface CellProps {
  index: number
  digit: string
  isActive: boolean
  isError: boolean
  primaryColor: string
  enteredAtRef: React.MutableRefObject<Map<number, number>>
  onPressCell: () => void
  onLongPressCell: () => void
  disabled: boolean
  variant?: 'light' | 'dark'
}

const Cell = memo(function Cell({
  index,
  digit,
  isActive,
  isError,
  primaryColor,
  enteredAtRef,
  onPressCell,
  onLongPressCell,
  disabled,
  variant = 'dark',
}: CellProps) {
  const isLight = variant === 'light'
  const idleBorder = isLight ? LIGHT_IDLE_BORDER : DARK_IDLE_BORDER
  const idleBackground = isLight ? LIGHT_IDLE_BACKGROUND : DARK_IDLE_BACKGROUND
  const textColor = isLight ? INK_PRIMARY : '#fff'
  
  const opacity = useSharedValue(0)
  const translateY = useSharedValue(DIGIT_TRANSLATE_Y)
  const borderProgress = useSharedValue(0)
  const errorPulse = useSharedValue(0)
  const cursorOpacity = useSharedValue(0)
  const lastRenderedDigitRef = useRef('')

  useEffect(() => {
    if (digit && !lastRenderedDigitRef.current) {
      const at = enteredAtRef.current.get(index) ?? Date.now()
      const wait = Math.max(0, at - Date.now())
      setTimeout(() => {
        haptic.light()
        opacity.value = withTiming(1, {
          duration: ENTRY_DURATION,
          easing: Easing.out(Easing.cubic),
        })
        translateY.value = withSpring(0, { damping: 18, stiffness: 260 })
      }, wait)
    } else if (!digit && lastRenderedDigitRef.current) {
      opacity.value = 0
      translateY.value = DIGIT_TRANSLATE_Y
    }
    lastRenderedDigitRef.current = digit
  }, [digit, index, enteredAtRef, opacity, translateY])

  useEffect(() => {
    if (isActive) {
      borderProgress.value = withSequence(
        withTiming(1, { duration: BORDER_PULSE_IN }),
        withDelay(
          BORDER_PULSE_HOLD,
          withTiming(0, { duration: BORDER_PULSE_OUT })
        )
      )
    }
  }, [isActive, borderProgress])

  useEffect(() => {
    if (isError) {
      errorPulse.value = withSequence(
        withTiming(1, { duration: 120 }),
        withTiming(0, { duration: 250 })
      )
    }
  }, [isError, errorPulse])

  useEffect(() => {
    if (isActive && !digit) {
      cursorOpacity.value = withRepeat(
        withSequence(
          withTiming(1, { duration: 0 }),
          withDelay(400, withTiming(0, { duration: 0 })),
          withTiming(0, { duration: 0 }),
          withDelay(400, withTiming(1, { duration: 0 }))
        ),
        -1,
        false
      )
    } else {
      cancelAnimation(cursorOpacity)
      cursorOpacity.value = 0
    }
    return () => {
      cancelAnimation(cursorOpacity)
    }
  }, [isActive, digit, cursorOpacity])

  const animatedStyle = useAnimatedStyle(() => {
    const blended = borderProgress.value > 0
      ? interpolateColor(
          borderProgress.value,
          [0, 1],
          [idleBorder, primaryColor]
        )
      : idleBorder
    const finalColor = errorPulse.value > 0
      ? interpolateColor(errorPulse.value, [0, 1], [blended, ERROR_BORDER])
      : blended
    return {
      borderColor: finalColor,
      borderWidth: 1.5,
    }
  })

  const digitStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    transform: [{ translateY: translateY.value }],
  }))

  const cursorStyle = useAnimatedStyle(() => ({
    opacity: cursorOpacity.value,
  }))

  return (
    <Pressable
      onPress={onPressCell}
      onLongPress={onLongPressCell}
      disabled={disabled}
      hitSlop={4}
      style={styles.cellWrapper}
    >
      <Reanimated.View style={[styles.cell, { backgroundColor: idleBackground }, animatedStyle]}>
        <Reanimated.View style={[styles.digitContainer, digitStyle]}>
          <Text color={textColor} fontSize={26} fontWeight="700" fontFamily="$heading">
            {digit}
          </Text>
        </Reanimated.View>
        {isActive && !digit && (
          <Reanimated.View style={[styles.cursorContainer, cursorStyle]}>
            <Text color={primaryColor} fontSize={26} fontWeight="700" fontFamily="$heading">
              |
            </Text>
          </Reanimated.View>
        )}
      </Reanimated.View>
    </Pressable>
  )
})

export const OtpInput = memo(function OtpInput({
  value,
  onChange,
  length = 6,
  onComplete,
  error = false,
  disabled = false,
  autoFocus = false,
  variant = 'dark',
}: OtpInputProps) {
  const { colorScheme } = useSettings()
  const inputRef = useRef<TextInput>(null)
  const shakeX = useSharedValue(0)
  const prevErrorRef = useRef(error)
  const prevLengthRef = useRef(value.length)

  useOtpCascade(value)
  const enteredAtRef = useRef<Map<number, number>>(new Map())

  useEffect(() => {
    if (prevErrorRef.current === false && error === true) {
      haptic.error()
      shakeX.value = withSequence(
        withTiming(-8, { duration: 50 }),
        withTiming(8, { duration: 50 }),
        withTiming(-6, { duration: 50 }),
        withTiming(6, { duration: 50 }),
        withTiming(-3, { duration: 50 }),
        withTiming(0, { duration: 50 })
      )
    }
    prevErrorRef.current = error
  }, [error, shakeX])

  // Held in a ref so the effect below can fire it without depending on its identity.
  // handleVerifyCode is rebuilt whenever the code or the auth state changes, and the effect
  // re-running mid-request is exactly what submitted the code twice.
  const onCompleteRef = useRef(onComplete)
  useEffect(() => {
    onCompleteRef.current = onComplete
  })

  /**
   * Fire onComplete once, on the transition to a full code.
   *
   * The guard used to read `prevLengthRef.current < length` but only wrote prevLengthRef on the
   * path where the code was *not* complete, so once it was, the condition stayed true forever.
   * Combined with depending on `onComplete` — whose identity changes on every auth state change
   * — a re-render while the request was in flight re-armed the timer and posted the code a
   * second time. Verification codes are single-use, so the first call signed the customer in
   * and the second came back 400 INVALID_CODE, which is the error they were shown.
   *
   * Now prevLengthRef is written on every run, so a re-render with an unchanged value cannot
   * re-arm it, and the effect no longer re-runs on identity changes at all.
   */
  useEffect(() => {
    const wasIncomplete = prevLengthRef.current < length
    prevLengthRef.current = value.length
    if (value.length !== length || !wasIncomplete) return

    const completedValue = value
    const timer = setTimeout(() => {
      onCompleteRef.current?.(completedValue)
    }, ON_COMPLETE_DELAY_MS)
    return () => clearTimeout(timer)
  }, [value, length])

  const handleChange = useCallback(
    (text: string) => {
      const digits = text.replace(/[^0-9]/g, '').slice(0, length)
      if (digits === value) return
      onChange(digits)
    },
    [length, onChange, value]
  )

  const focusInput = useCallback(() => {
    haptic.selection()
    inputRef.current?.focus()
  }, [])

  const handleLongPressPaste = useCallback(async () => {
    try {
      const text = await Clipboard.getStringAsync()
      const digits = text.replace(/[^0-9]/g, '').slice(0, length)
      if (digits.length > 0) {
        haptic.medium()
        onChange(digits)
      }
    } catch {
      // Clipboard read failed silently
    }
  }, [length, onChange])

  useEffect(() => {
    if (autoFocus) {
      const t = setTimeout(focusInput, 200)
      return () => clearTimeout(t)
    }
  }, [autoFocus, focusInput])

  const rowAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: shakeX.value }],
  }))

  const cells: React.ReactNode[] = []
  for (let i = 0; i < length; i++) {
    const isActive = i === value.length && !disabled
    cells.push(
      <Cell
        key={i}
        index={i}
        digit={value[i] ?? ''}
        isActive={isActive}
        isError={error}
        primaryColor={colorScheme.primary}
        enteredAtRef={enteredAtRef}
        onPressCell={focusInput}
        onLongPressCell={handleLongPressPaste}
        disabled={disabled}
        variant={variant}
      />
    )
  }

  const hiddenInputStyle: any = isWeb
    ? {
        position: 'absolute',
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        width: '100%',
        height: '100%',
        opacity: 0,
        color: 'transparent',
        caretColor: 'transparent',
        textIndent: -9999,
        outline: 'none',
        borderWidth: 0,
        backgroundColor: 'transparent',
        zIndex: 2,
        cursor: 'default',
      }
    : {
        position: 'absolute',
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        width: '100%',
        height: '100%',
        opacity: 0,
        color: 'transparent',
        backgroundColor: 'transparent',
      }

  return (
    <View style={styles.container}>
      <Reanimated.View style={[styles.cellsLayer, rowAnimatedStyle]}>
        <XStack
          gap={CELL_GAP}
          justifyContent="center"
          alignItems="center"
          style={styles.row}
        >
          {cells}
        </XStack>
      </Reanimated.View>

      <TextInput
        ref={inputRef}
        value={value}
        onChangeText={handleChange}
        editable={!disabled}
        autoFocus={autoFocus}
        caretHidden
        selectTextOnFocus={false}
        style={[StyleSheet.absoluteFill, hiddenInputStyle]}
        {...numberInputProps()}
        {...otpAutofillProps()}
      />
    </View>
  )
})

const styles = StyleSheet.create({
  container: {
    width: '100%',
    position: 'relative',
  },
  cellsLayer: {
    width: '100%',
  },
  row: {
    width: '100%',
  },
  cellWrapper: {
    width: CELL_SIZE,
    height: 56,
  },
  cell: {
    width: CELL_SIZE,
    height: 56,
    borderRadius: 14,
    borderWidth: 1.5,
    overflow: 'hidden',
    alignItems: 'center',
    justifyContent: 'center',
  },
  digitContainer: {
    alignItems: 'center',
    justifyContent: 'center',
    width: '100%',
    height: '100%',
  },
  cursorContainer: {
    position: 'absolute',
    alignItems: 'center',
    justifyContent: 'center',
    width: '100%',
    height: '100%',
  },
})
