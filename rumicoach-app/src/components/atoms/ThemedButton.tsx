import { type ReactNode, useCallback, useEffect } from 'react'
import { Pressable, StyleProp, StyleSheet, ViewStyle } from 'react-native'
import { Button, type ButtonProps } from 'tamagui'
import { useSettings } from '@/hooks/useSettings'
import { GLASS, INK } from '@/styles/glass'
import { ThemedSpinner } from './ThemedSpinner'
import * as Haptics from 'expo-haptics'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  withRepeat,
  Easing,
  interpolate,
} from 'react-native-reanimated'

interface ThemedButtonProps extends Omit<ButtonProps, 'variant'> {
  variant?: 'solid' | 'glass' | 'outline' | 'ghost' | 'error'
  fullWidth?: boolean
  loading?: boolean
  glow?: boolean
  children: ReactNode
  flex?: number | undefined
  buttonStyle?: StyleProp<ViewStyle>
}

export function ThemedButton({
  variant = 'solid',
  fullWidth = false,
  loading = false,
  glow = false,
  children,
  disabled,
  style,
  onPress,
  flex = undefined,
  buttonStyle,
  ...props
}: ThemedButtonProps) {
  const { colorScheme } = useSettings()
  const scale = useSharedValue(1)
  const opacity = useSharedValue(1)
  const glowProgress = useSharedValue(0)

  useEffect(() => {
    if (glow && !disabled && !loading) {
      glowProgress.value = withRepeat(
        withTiming(1, { duration: 1800, easing: Easing.inOut(Easing.sin) }),
        -1,
        true
      )
    } else {
      glowProgress.value = 0
    }
  }, [glow, disabled, loading, glowProgress])

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
    opacity: opacity.value,
  }))

  const glowStyle = useAnimatedStyle(() => {
    const radius = interpolate(glowProgress.value, [0, 1], [8, 14])
    return {
      boxShadow: `0px 0px ${radius}px ${colorScheme.primary}59`,
    } as any
  })

  const handlePressIn = useCallback(() => {
    if (!disabled && !loading) {
      scale.value = withSpring(0.97, { damping: 15, stiffness: 400 })
      opacity.value = withTiming(0.8, { duration: 100 })
      Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
    }
  }, [scale, opacity, disabled, loading])

  const handlePressOut = useCallback(() => {
    scale.value = withSpring(1, { damping: 15, stiffness: 400 })
    opacity.value = withTiming(1, { duration: 150 })
  }, [scale, opacity])

  const getStyles = () => {
    const base: Record<string, unknown> = {
      borderRadius: 8,
      width: fullWidth ? '100%' : 'unset',
      cursor: disabled || loading ? 'not-allowed' : 'pointer',
      ...buttonStyle
    }
    switch (variant) {
      case 'glass':
        // Frosted light-glass pill with dark ink — matches the app material.
        return {
          ...base,
          backgroundColor: 'rgba(255,255,255,0.75)',
          borderWidth: GLASS.borderWidth,
          borderColor: GLASS.borderColor,
          color: INK.primary,
        }
      case 'solid':
        // primary (not accent): every scheme's primary keeps white label ≥7:1.
        return {
          ...base,
          backgroundColor: colorScheme.primary,
          color: '#fff',
        }
      case 'outline':
        return {
          ...base,
          backgroundColor: 'transparent',
          borderWidth: 1,
          borderColor: colorScheme.accent,
          color: colorScheme.accent,
        }
      case 'ghost':
        return {
          ...base,
          backgroundColor: 'transparent',
          color: colorScheme.accent,
        }
      case 'error':
        return {
          ...base,
          backgroundColor: '#FEE2E2',
          color: '#B91C1C',
        }
      default:
        return base
    }
  }

  return (
    <Pressable onPressIn={handlePressIn} onPressOut={handlePressOut} style={{ flex: flex }}>
      {glow && (
        <Reanimated.View
          pointerEvents="none"
          style={[styles.glowLayer, glowStyle]}
        />
      )}
      <Reanimated.View style={animatedStyle}>
        <Button
          size="$5"
          disabled={disabled || loading}
          {...getStyles()}
          style={[{ opacity: disabled ? 0.5 : 1 }, style as Record<string, unknown>]}
          onPress={onPress}
          {...props}
        >
          {loading ? <ThemedSpinner color="white" /> : children}
        </Button>
      </Reanimated.View>
    </Pressable >
  )
}

const styles = StyleSheet.create({
  glowLayer: {
    position: 'absolute',
    left: 0,
    right: 0,
    top: 0,
    bottom: 0,
    borderRadius: 9999,
  },
})
