import { type ReactNode, useCallback } from 'react'
import { Pressable, type PressableProps, View, StyleSheet } from 'react-native'
import { BlurView } from 'expo-blur'
import { useSettings } from '@/hooks/useSettings'
import { GLASS } from '@/styles/glass'
import * as Haptics from 'expo-haptics'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
} from 'react-native-reanimated'

type ButtonVariant = 'glass' | 'solid' | 'outline' | 'ghost' | 'error'
type ButtonSize = 'sm' | 'md' | 'lg'

interface ThemedIconButtonProps extends PressableProps {
  children: ReactNode
  variant?: ButtonVariant
  size?: ButtonSize
}

const sizeMap: Record<ButtonSize, { size: number }> = {
  sm: { size: 28 },
  md: { size: 36 },
  lg: { size: 44 },
}

export function ThemedIconButton({ children, variant = 'glass', size = 'md', style, disabled, ...props }: ThemedIconButtonProps) {
  const { colorScheme } = useSettings()
  const { size: buttonSize } = sizeMap[size]
  const borderRadius = buttonSize / 2

  const scale = useSharedValue(1)
  const opacity = useSharedValue(1)

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
    opacity: opacity.value,
  }))

  const handlePressIn = useCallback(() => {
    if (!disabled) {
      scale.value = withSpring(0.88, { damping: 15, stiffness: 400 })
      opacity.value = withTiming(0.75, { duration: 100 })
      Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
    }
  }, [scale, opacity, disabled])

  const handlePressOut = useCallback(() => {
    scale.value = withSpring(1, { damping: 15, stiffness: 400 })
    opacity.value = withTiming(1, { duration: 150 })
  }, [scale, opacity])

  const bgColor = () => {
    switch (variant) {
      case 'glass': return 'transparent'
      case 'solid': return colorScheme.primary
      case 'outline': return 'transparent'
      case 'ghost': return 'transparent'
      case 'error': return '#ba1a1a'
    }
  }

  const borderColor = () => {
    switch (variant) {
      case 'glass': return GLASS.borderColor
      case 'outline': return colorScheme.primary
      default: return 'transparent'
    }
  }

  return (
    <Pressable
      hitSlop={size === 'sm' ? 8 : 4}
      accessibilityRole="button"
      disabled={disabled}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      style={[{ opacity: disabled ? 0.5 : 1 }, style as any]}
      {...props}
    >
      <Reanimated.View
        style={[
          animatedStyle,
          {
            width: buttonSize,
            height: buttonSize,
            borderRadius,
            backgroundColor: bgColor(),
            borderWidth: variant === 'outline' || variant === 'glass' ? 1 : 0,
            borderColor: borderColor(),
            overflow: 'hidden',
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
          },
        ]}
      >
        {variant === 'glass' ? (
          <>
            <BlurView
              tint={GLASS.tint}
              intensity={GLASS.intensity}
              style={[StyleSheet.absoluteFill, { pointerEvents: 'none' }]}
            />
            <View style={[StyleSheet.absoluteFill, { pointerEvents: 'none', backgroundColor: GLASS.baseFill }]} />
          </>
        ) : null}
        {/* Own stacking layer: on web, static children (SVG icons) would other-
            wise paint BELOW the absolutely-positioned frost overlays. */}
        <View style={styles.content}>{children}</View>
      </Reanimated.View>
    </Pressable>
  )
}

const styles = StyleSheet.create({
  content: {
    position: 'relative',
    zIndex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 99
  },
})
