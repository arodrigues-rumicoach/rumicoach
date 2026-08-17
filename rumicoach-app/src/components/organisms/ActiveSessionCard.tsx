import { useEffect } from 'react'
import type { ChatStatus } from '@/context/SessionContext'
import OrganicVisualizer from '@/components/organisms/OrganicVisualizer'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import { StatusIndicator } from '@/components/molecules/StatusIndicator'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  interpolate,
  Extrapolation,
} from 'react-native-reanimated'

interface ActiveSessionCardProps {
  status: ChatStatus
  inputVolume: number
  onStop: () => void
}

export function ActiveSessionCard({ status, inputVolume, onStop }: ActiveSessionCardProps) {
  const isEnding = status === 'ending'
  const endingProgress = useSharedValue(0)

  useEffect(() => {
    endingProgress.value = withTiming(isEnding ? 1 : 0, { duration: isEnding ? 500 : 200 })
  }, [isEnding, endingProgress])

  const containerStyle = useAnimatedStyle(() => ({
    transform: [{
      scale: interpolate(endingProgress.value, [0, 1], [1, 0.85], Extrapolation.CLAMP),
    }],
    opacity: interpolate(endingProgress.value, [0, 1], [1, 0], Extrapolation.CLAMP),
    width: '100%'
  }))

  return (
    <Reanimated.View style={containerStyle}>
      <GlassPanel variant="light">
        <OrganicVisualizer status={status} inputVolume={inputVolume} />
        <StatusIndicator status={status} />
      </GlassPanel>
    </Reanimated.View>
  )
}
