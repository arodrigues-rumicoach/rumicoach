import { useEffect } from 'react'
import { View, type ViewStyle } from 'react-native'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  interpolate,
  Extrapolation,
  useAnimatedReaction,
  runOnJS,
} from 'react-native-reanimated'
import { useSettings } from '@/hooks/useSettings'
import type { ChatStatus } from '@/context/SessionContext'

interface OrganicVisualizerProps {
  status: ChatStatus
  inputVolume: number
}

const SPRING_CONFIG = {
  damping: 12,
  stiffness: 180,
  mass: 0.6,
}

export default function OrganicVisualizer({ status, inputVolume }: OrganicVisualizerProps) {
  const { colorScheme } = useSettings()
  const isConnected = status !== 'disconnected'
  const isAISpeaking = status === 'speaking'
  const isAIThinking = status === 'thinking'
  const isListening = status === 'listening'
  const isEnding = status === 'ending'

  const modeScale = useSharedValue(1)
  const glowOpacity = useSharedValue(0)
  const rippleScale = useSharedValue(1)
  const rippleOpacity = useSharedValue(0)
  const coreOpacity = useSharedValue(0.8)
  const inputVolumeShared = useSharedValue(0)
  const frameCounter = useSharedValue(0)

  // Update inputVolume on UI thread via worklet
  useEffect(() => {
    inputVolumeShared.value = inputVolume
  }, [inputVolume])

  // Continuous animation loop - triggers on every frame via frameCounter
  // The frameCounter increments continuously, causing this reaction to fire on every frame
  useAnimatedReaction(
    () => {
      frameCounter.value
      return {
        vol: inputVolumeShared.value,
        timestamp: Date.now(),
      }
    },
    (current) => {
      const vol = Math.max(0, Math.min(1, current.vol))
      const timestamp = current.timestamp
      const breath = 1 + 0.015 * Math.sin(timestamp * 0.0018)

      if (!isConnected) {
        modeScale.value = withSpring(1, SPRING_CONFIG)
        glowOpacity.value = withSpring(0, SPRING_CONFIG)
        rippleScale.value = withSpring(1, SPRING_CONFIG)
        rippleOpacity.value = withSpring(0, SPRING_CONFIG)
      } else if (isEnding) {
        modeScale.value = withSpring(0.6, { ...SPRING_CONFIG, stiffness: 60 })
        glowOpacity.value = withSpring(0.1, { ...SPRING_CONFIG, stiffness: 60 })
        rippleScale.value = withSpring(1.2, { ...SPRING_CONFIG, stiffness: 60 })
        rippleOpacity.value = withSpring(0, { ...SPRING_CONFIG, stiffness: 60 })
      } else if (isAISpeaking) {
        const t = timestamp * 0.001
        const p1 = Math.sin(t * 1.2)
        const p2 = Math.sin(t * 2.7 + 1.1)
        const p3 = Math.sin(t * 0.6 + 2.3)
        const combined = p1 * 0.5 + p2 * 0.3 + p3 * 0.2
        const pulse = 0.5 + 0.5 * combined
        const target = 1 + 0.25 * pulse
        modeScale.value = withSpring(target, SPRING_CONFIG)
        glowOpacity.value = withSpring(0.5 + 0.4 * pulse, { ...SPRING_CONFIG, stiffness: 80 })
        rippleScale.value = withSpring(1 + 0.3 * pulse, SPRING_CONFIG)
        rippleOpacity.value = withSpring(0.15 + 0.2 * pulse, { ...SPRING_CONFIG, stiffness: 80 })
      } else if (isAIThinking) {
        const target = 1 + 0.1 * (0.5 + 0.5 * Math.sin(timestamp * 0.015))
        modeScale.value = withSpring(target, SPRING_CONFIG)
        glowOpacity.value = withSpring(0.5, { ...SPRING_CONFIG, stiffness: 80 })
        rippleScale.value = withSpring(1, SPRING_CONFIG)
        rippleOpacity.value = withSpring(0, SPRING_CONFIG)
      } else if (isListening) {
        const bounce = 1 + vol * 0.6
        const target = breath * bounce
        modeScale.value = withSpring(target, { ...SPRING_CONFIG, stiffness: 220, damping: 10 })
        glowOpacity.value = withSpring(0.35, { ...SPRING_CONFIG, stiffness: 160 })
        rippleScale.value = withSpring(1, SPRING_CONFIG)
        rippleOpacity.value = withSpring(0, SPRING_CONFIG)
      } else {
        modeScale.value = withSpring(1, SPRING_CONFIG)
        glowOpacity.value = withSpring(0.2, { ...SPRING_CONFIG, stiffness: 80 })
        rippleScale.value = withSpring(1, SPRING_CONFIG)
        rippleOpacity.value = withSpring(0, SPRING_CONFIG)
      }

      coreOpacity.value = withTiming(isConnected ? 1 : 0.8, { duration: 200 })
    }
  )

  // Trigger continuous animation by incrementing frameCounter
  // This runs on JS thread but only to trigger the worklet - all animation logic is on UI thread
  useEffect(() => {
    let frameId: number
    const tick = () => {
      frameCounter.value = Date.now()
      frameId = requestAnimationFrame(tick)
    }
    frameId = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frameId)
  }, [frameCounter])

  const circleSize = 120

  const rippleStyle = useAnimatedStyle(() => ({
    transform: [{ scale: rippleScale.value }],
    opacity: rippleOpacity.value,
  }))

  const glowStyle = useAnimatedStyle(() => {
    const s = 1.1 + (modeScale.value - 1) * 1.4
    return {
      transform: [{ scale: s }],
      opacity: glowOpacity.value,
    }
  })

  const coreStyle = useAnimatedStyle(() => ({
    transform: [{ scale: modeScale.value }],
    opacity: coreOpacity.value,
    backgroundColor: isAISpeaking ? colorScheme.primary : colorScheme.secondary,
    boxShadow: `0px 0px ${isEnding ? 10 : isAISpeaking ? 30 : 20}px ${isAISpeaking ? colorScheme.primary : colorScheme.secondary}${isEnding ? '33' : isAISpeaking ? 'b3' : '99'}`,
  }))

  const baseStyle: ViewStyle = {
    position: 'absolute',
    width: circleSize,
    height: circleSize,
    borderRadius: circleSize / 2,
  }

  return (
    <View style={{
      alignItems: 'center',
      justifyContent: 'center',
      width: 180,
      height: 180,
      position: 'relative',
    }}>
      <Reanimated.View
        style={[baseStyle, {
          backgroundColor: 'rgba(255,255,255,0.12)',
          zIndex: 0,
        }, rippleStyle]}
      />
      <Reanimated.View
        style={[baseStyle, {
          borderWidth: 2,
          borderColor: 'rgba(255,255,255,0.35)',
          zIndex: 10,
        }, glowStyle]}
      />
      <Reanimated.View
        style={[baseStyle, {
          zIndex: 20,
        }, coreStyle]}
      />
    </View>
  )
}
