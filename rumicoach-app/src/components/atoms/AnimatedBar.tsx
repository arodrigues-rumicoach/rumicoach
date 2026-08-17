import { useEffect } from 'react'
import { View, StyleSheet } from 'react-native'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withDelay,
  withSpring,
} from 'react-native-reanimated'

const STAGGER_DELAY_MS = 120
const SPRING_CONFIG = { damping: 20, stiffness: 120 }

interface AnimatedBarProps {
  /** Target width as a percentage (0–100). */
  targetPercent: number
  /** Colour of the fill bar. */
  color: string
  /** Delay in ms before the animation starts (multiplied by index internally). */
  delayMs?: number
}

export function AnimatedBar({ targetPercent, color, delayMs = STAGGER_DELAY_MS }: AnimatedBarProps) {
  const progress = useSharedValue(0)

  useEffect(() => {
    progress.value = withDelay(delayMs, withSpring(targetPercent, SPRING_CONFIG))
  }, [delayMs, targetPercent, progress])

  const animatedFill = useAnimatedStyle(() => ({
    width: `${progress.value}%`,
  }))

  return (
    <View style={styles.track}>
      <Reanimated.View style={[styles.fill, { backgroundColor: color }, animatedFill]} />
    </View>
  )
}

const styles = StyleSheet.create({
  track: {
    height: 6,
    borderRadius: 3,
    backgroundColor: 'rgba(0,0,0,0.10)',
    overflow: 'hidden',
  },
  fill: {
    height: '100%',
    borderRadius: 3,
  },
})
