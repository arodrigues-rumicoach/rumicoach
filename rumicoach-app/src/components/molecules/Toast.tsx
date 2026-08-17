import { useEffect, useRef } from 'react'
import { Platform, StyleSheet } from 'react-native'
import { Text } from '@tamagui/core'
import { useSettings } from '@/hooks/useSettings'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  withDelay,
  runOnJS,
  useAnimatedReaction,
} from 'react-native-reanimated'

interface ToastProps {
  message: string
  type?: 'success' | 'error'
  onClose: () => void
  duration?: number
}

export function Toast({ message, type = 'success', onClose, duration = 3000 }: ToastProps) {
  const { colorScheme } = useSettings()
  const opacity = useSharedValue(0)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    opacity.value = 0

    opacity.value = withDelay(
      50,
      withTiming(1, { duration: 200 }, (finished) => {
        if (finished) {
          opacity.value = withDelay(
            duration,
            withTiming(0, { duration: 200 }, (finished) => {
              if (finished) {
                runOnJS(onCloseRef.current)()
              }
            })
          )
        }
      })
    )
  }, [message, duration])

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
  }))

  const bgColor = type === 'success' ? '#047857' : '#dc2626'

  return (
    <Reanimated.View style={[styles.container, animatedStyle, { backgroundColor: bgColor }]}>
      <Text style={styles.text}>{message}</Text>
    </Reanimated.View>
  )
}

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    top: 60,
    left: 16,
    right: 16,
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 12,
    zIndex: 1000,
    alignItems: 'center',
  },
  text: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600',
  },
})
