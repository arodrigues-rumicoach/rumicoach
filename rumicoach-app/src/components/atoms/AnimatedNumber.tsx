import { useState, useEffect } from 'react'
import {
  useSharedValue,
  useAnimatedReaction,
  withDelay,
  withSpring,
  runOnJS,
} from 'react-native-reanimated'

const STAGGER_DELAY_MS = 120
const SPRING_CONFIG = { damping: 18, stiffness: 100 }

interface AnimatedNumberProps {
  /** Target number to animate to. */
  value: number
  /** Delay in ms before the animation starts. */
  delayMs?: number
  /** Render function that receives the current animated number. */
  children: (displayValue: number) => React.ReactNode
}

export function AnimatedNumber({ value, delayMs = STAGGER_DELAY_MS, children }: AnimatedNumberProps) {
  const [display, setDisplay] = useState(0)
  const shared = useSharedValue(0)

  useEffect(() => {
    shared.value = withDelay(delayMs, withSpring(value, SPRING_CONFIG))
  }, [delayMs, value, shared])

  useAnimatedReaction(
    () => shared.value,
    (current) => {
      runOnJS(setDisplay)(Math.round(current))
    },
  )

  return <>{children(display)}</>
}
