import { useRef, useCallback, useEffect } from 'react'
import { LayoutChangeEvent, Dimensions } from 'react-native'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  useAnimatedReaction,
  withSpring,
  withDelay,
  FadeIn,
  FadeOut,
  runOnJS,
} from 'react-native-reanimated'
import type { ViewStyle, StyleProp } from 'react-native'
import { useScrollNav } from '@/context/ScrollNavContext'

interface AnimatedCardProps {
  children: React.ReactNode
  isVisible?: boolean
  viewportAware?: boolean
  staggerIndex?: number
  style?: StyleProp<ViewStyle>
}

const STAGGER_DELAY_MS = 120
const VIEWPORT_TRIGGER_RATIO = 0.85
const SPRING_CONFIG = { damping: 18, stiffness: 180 }

export function AnimatedCard({
  children,
  isVisible = true,
  viewportAware = false,
  staggerIndex = 0,
  style,
}: AnimatedCardProps) {
  const { scrollY } = useScrollNav()
  const hasAnimated = useRef(false)
  const layoutY = useSharedValue(0)
  const layoutHeight = useSharedValue(0)
  const opacity = useSharedValue(viewportAware ? 0 : 1)
  const translateY = useSharedValue(viewportAware ? 24 : 0)
  const windowHeightRef = useRef(Dimensions.get('window').height)

  const triggerAnimation = useCallback(() => {
    if (hasAnimated.current) return
    hasAnimated.current = true
    const delay = staggerIndex * STAGGER_DELAY_MS

    opacity.value = withDelay(delay, withSpring(1, SPRING_CONFIG))
    translateY.value = withDelay(delay, withSpring(0, SPRING_CONFIG))
  }, [opacity, translateY, staggerIndex])

  const handleLayout = useCallback((e: LayoutChangeEvent) => {
    layoutY.value = e.nativeEvent.layout.y
    layoutHeight.value = e.nativeEvent.layout.height
  }, [layoutY, layoutHeight])

  useAnimatedReaction(
    () => ({
      scroll: scrollY.value,
      y: layoutY.value,
      h: layoutHeight.value,
    }),
    (current) => {
      if (!viewportAware || hasAnimated.current) return
      if (current.h === 0) return

      const triggerPoint = current.scroll + windowHeightRef.current * VIEWPORT_TRIGGER_RATIO

      if (current.y < triggerPoint) {
        runOnJS(triggerAnimation)()
      }
    },
  )

  // Fallback: ensure animation triggers on mount for elements already in viewport
  useEffect(() => {
    if (!viewportAware) return

    const timer = setTimeout(() => {
      if (!hasAnimated.current && layoutHeight.value > 0 && layoutY.value < windowHeightRef.current * VIEWPORT_TRIGGER_RATIO) {
        triggerAnimation()
      }
    }, 100)

    return () => clearTimeout(timer)
  }, [viewportAware, layoutHeight, layoutY, triggerAnimation])

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    transform: [{ translateY: translateY.value }],
  }))

  if (viewportAware) {
    return (
      <Reanimated.View
        onLayout={handleLayout}
        style={[
          style,
          { overflow: 'hidden' },
          animatedStyle,
        ]}
      >
        {children}
      </Reanimated.View>
    )
  }

  return (
    <Reanimated.View
      entering={FadeIn.duration(300).springify().damping(20).stiffness(200)}
      exiting={FadeOut.duration(200)}
      style={[
        style,
        { overflow: 'hidden' },
      ]}
    >
      {children}
    </Reanimated.View>
  )
}
