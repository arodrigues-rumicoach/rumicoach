import { useEffect } from 'react'
import { XStack, Text } from 'tamagui'
import i18n from '@/i18n'
import { INK } from '@/styles/glass'
import type { ChatStatus } from '@/context/SessionContext'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withRepeat,
  withTiming,
  withSequence,
  withDelay,
  interpolate,
  Extrapolation,
  cancelAnimation,
} from 'react-native-reanimated'

interface StatusMetaEntry {
  color: string
  labelKey: string
  fallback: string
  pulse: boolean
}

const STATUS_META: Partial<Record<ChatStatus, StatusMetaEntry>> = {
  connecting: { color: '#6b7280', labelKey: 'connecting', fallback: 'Connecting...', pulse: false },
  preparing: { color: '#6b7280', labelKey: 'processing', fallback: 'Preparing...', pulse: false },
  listening: { color: '#3b82f6', labelKey: 'listening', fallback: 'Listening...', pulse: true },
  thinking: { color: '#f59e0b', labelKey: 'processing', fallback: 'Thinking...', pulse: true },
  speaking: { color: '#22c55e', labelKey: 'rumi_is_speaking', fallback: 'Rumi is speaking...', pulse: true },
  retrying: { color: '#6b7280', labelKey: 'retrying', fallback: 'Retrying...', pulse: false },
  ending: { color: '#ef4444', labelKey: 'ending_session', fallback: 'Ending session...', pulse: false },
}

export function StatusIndicator({ status }: { status: ChatStatus }) {
  const meta = STATUS_META[status]
  const pulseScale = useSharedValue(1)
  const pulseOpacity = useSharedValue(1)

  useEffect(() => {
    if (meta?.pulse) {
      pulseScale.value = withRepeat(
        withSequence(
          withTiming(1.5, { duration: 800 }),
          withTiming(1, { duration: 800 })
        ),
        -1,
        false
      )
      pulseOpacity.value = withRepeat(
        withSequence(
          withTiming(0.3, { duration: 800 }),
          withTiming(1, { duration: 800 })
        ),
        -1,
        false
      )
    } else {
      cancelAnimation(pulseScale)
      cancelAnimation(pulseOpacity)
      pulseScale.value = 1
      pulseOpacity.value = 1
    }

    return () => {
      cancelAnimation(pulseScale)
      cancelAnimation(pulseOpacity)
    }
  }, [meta?.pulse])

  const pulseStyle = useAnimatedStyle(() => ({
    transform: [{ scale: pulseScale.value }],
    opacity: pulseOpacity.value,
  }))

  if (!meta) return null

  return (
    <XStack
      backgroundColor="rgba(0,0,0,0.06)"
      borderRadius={9999}
      borderWidth={1}
      borderColor="rgba(0,0,0,0.10)"
      gap={8}
      justifyContent="center"
      alignItems="center"
      padding={8}
    >
      <Reanimated.View style={[{ position: 'relative' }, pulseStyle]}>
        <Reanimated.View
          style={{
            position: 'absolute',
            width: 8,
            height: 8,
            borderRadius: 4,
            backgroundColor: meta.color,
            opacity: 0.5,
          }}
        />
        <Reanimated.View
          style={{
            width: 8,
            height: 8,
            borderRadius: 4,
            backgroundColor: meta.color,
          }}
        />
      </Reanimated.View>
      <Text color={INK.primary} fontSize={13} fontWeight="500">
        {i18n.t(meta.labelKey) || meta.fallback}
      </Text>
    </XStack>
  )
}
