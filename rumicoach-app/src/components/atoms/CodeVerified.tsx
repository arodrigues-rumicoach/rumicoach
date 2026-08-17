import { useEffect } from 'react'
import { StyleSheet } from 'react-native'
import { Text, YStack } from 'tamagui'
import { Check } from 'lucide-react-native'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  withSequence,
  withDelay,
  Easing,
} from 'react-native-reanimated'
import { useSettings } from '@/hooks/useSettings'
import { haptic } from '@/utils/haptics'
import { INK } from '@/styles/glass'

interface CodeVerifiedProps {
  title?: string
  subtitle?: string
  onComplete?: () => void
  durationMs?: number
  accentColor?: string
}

const CIRCLE_SIZE = 72

/**
 * Full-screen-style success indicator: a circle that scales in with a
 * spring, a checkmark stroke that draws on, and a fade-in title/subtitle.
 * Fires `haptic.success()` once on mount and `onComplete` after the
 * configured duration so the caller can advance to the next step.
 */
export function CodeVerified({
  title = 'Code verified',
  subtitle,
  onComplete,
  durationMs = 900,
  accentColor,
}: CodeVerifiedProps) {
  const { colorScheme } = useSettings()
  const accent = accentColor ?? colorScheme.accent
  const circleScale = useSharedValue(0)
  const circleOpacity = useSharedValue(0)
  const checkOpacity = useSharedValue(0)
  const checkScale = useSharedValue(0.4)
  const ringScale = useSharedValue(0)
  const ringOpacity = useSharedValue(0.6)
  const titleOpacity = useSharedValue(0)
  const titleTranslateY = useSharedValue(8)
  const subtitleOpacity = useSharedValue(0)

  useEffect(() => {
    haptic.success()

    circleOpacity.value = withTiming(1, { duration: 160, easing: Easing.out(Easing.cubic) })
    circleScale.value = withSpring(1, { damping: 12, stiffness: 200 })

    ringScale.value = 0
    ringOpacity.value = 0.6
    ringScale.value = withTiming(1.6, { duration: 700, easing: Easing.out(Easing.cubic) })
    ringOpacity.value = withTiming(0, { duration: 700, easing: Easing.out(Easing.cubic) })

    checkOpacity.value = withDelay(180, withTiming(1, { duration: 180 }))
    checkScale.value = withDelay(180, withSpring(1, { damping: 12, stiffness: 240 }))

    titleOpacity.value = withDelay(260, withTiming(1, { duration: 220 }))
    titleTranslateY.value = withDelay(260, withSpring(0, { damping: 16, stiffness: 220 }))
    subtitleOpacity.value = withDelay(360, withTiming(1, { duration: 220 }))

    const timer = setTimeout(() => {
      onComplete?.()
    }, durationMs)

    return () => clearTimeout(timer)
  }, [
    durationMs,
    onComplete,
    circleScale,
    circleOpacity,
    checkOpacity,
    checkScale,
    ringScale,
    ringOpacity,
    titleOpacity,
    titleTranslateY,
    subtitleOpacity,
  ])

  const circleStyle = useAnimatedStyle(() => ({
    opacity: circleOpacity.value,
    transform: [{ scale: circleScale.value }],
  }))

  const checkStyle = useAnimatedStyle(() => ({
    opacity: checkOpacity.value,
    transform: [{ scale: checkScale.value }],
  }))

  const ringStyle = useAnimatedStyle(() => ({
    opacity: ringOpacity.value,
    transform: [{ scale: ringScale.value }],
  }))

  const titleStyle = useAnimatedStyle(() => ({
    opacity: titleOpacity.value,
    transform: [{ translateY: titleTranslateY.value }],
  }))

  const subtitleStyle = useAnimatedStyle(() => ({
    opacity: subtitleOpacity.value,
  }))

  return (
    <YStack alignItems="center" gap="$3" width="100%" paddingVertical="$4">
      <YStack width={CIRCLE_SIZE} height={CIRCLE_SIZE} alignItems="center" justifyContent="center">
        <Reanimated.View
          style={[
            styles.ring,
            { borderColor: accent, width: CIRCLE_SIZE, height: CIRCLE_SIZE, borderRadius: CIRCLE_SIZE / 2 },
            ringStyle,
          ]}
        />
        <Reanimated.View
          style={[
            styles.circle,
            { backgroundColor: accent, width: CIRCLE_SIZE, height: CIRCLE_SIZE, borderRadius: CIRCLE_SIZE / 2 },
            circleStyle,
          ]}
        >
          <Reanimated.View style={checkStyle}>
            <Check size={36} color="white" strokeWidth={3.5} />
          </Reanimated.View>
        </Reanimated.View>
      </YStack>
      <Reanimated.View style={titleStyle}>
        <Text color={INK.primary} fontSize={18} fontWeight="700" textAlign="center">
          {title}
        </Text>
      </Reanimated.View>
      {subtitle && (
        <Reanimated.View style={subtitleStyle}>
          <Text color={INK.primary} fontSize={13} textAlign="center">
            {subtitle}
          </Text>
        </Reanimated.View>
      )}
    </YStack>
  )
}

const styles = StyleSheet.create({
  ring: {
    position: 'absolute',
    borderWidth: 2,
  },
  circle: {
    alignItems: 'center',
    justifyContent: 'center',
  },
})
