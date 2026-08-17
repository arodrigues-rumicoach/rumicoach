import { useEffect } from 'react'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  interpolate,
  Extrapolation,
} from 'react-native-reanimated'
import type { ChatStatus } from '../context/SessionContext'

const GLOW_LEVELS: Record<string, number> = {
  disconnected: 0,
  connecting: 0,
  preparing: 0,
  listening: 1,
  speaking: 2,
  thinking: 3,
  retrying: 4,
}

const GLOW_COLORS = [
  'rgba(0,0,0,0)',
  'rgba(59,130,246,0.06)',
  'rgba(34,197,94,0.06)',
  'rgba(245,158,11,0.06)',
  'rgba(245,158,11,0.06)',
]

export function useStatusGlow(status: ChatStatus, duration = 600) {
  const glowAnim = useSharedValue(0)

  useEffect(() => {
    glowAnim.value = withTiming(GLOW_LEVELS[status] ?? 0, { duration })
  }, [status, duration])

  const animatedStyle = useAnimatedStyle(() => {
    const colorIndex = Math.round(
      interpolate(glowAnim.value, [0, 1, 2, 3, 4], [0, 1, 2, 3, 4], Extrapolation.CLAMP)
    )
    return {
      backgroundColor: GLOW_COLORS[colorIndex] || GLOW_COLORS[0],
    }
  })

  return animatedStyle
}
