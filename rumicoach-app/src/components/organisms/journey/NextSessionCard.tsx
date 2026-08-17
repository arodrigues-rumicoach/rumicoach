import { memo, useCallback } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Lock, ArrowRight, Mic } from 'lucide-react-native'
import i18n from '@/i18n'
import { getSessionDetails } from './constants'
import { GlassCard } from '@/components/atoms'
import { INK } from '@/styles/glass'
import { haptic } from '@/utils/haptics'
import type { NextSessionInfo } from '@/api'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated'

interface NextSessionCardProps {
  nextSession: NextSessionInfo
  onStart?: (session: NextSessionInfo['session']) => void
  blurTargetRef?: React.RefObject<View | null>
}

// availabilityText renders when the next chapter unlocks. Gates are calendar-day
// based (the backend anchors them to local midnight), so the copy speaks in days:
// "Available tomorrow", then "Available in X days".
function availabilityText(availableAt: string): string {
  const unlock = new Date(availableAt)
  const now = new Date()
  if (!Number.isFinite(unlock.getTime()) || unlock.getTime() <= now.getTime()) {
    return i18n.t('journey_available_now') || 'Available now'
  }
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const startOfUnlockDay = new Date(unlock.getFullYear(), unlock.getMonth(), unlock.getDate()).getTime()
  const days = Math.round((startOfUnlockDay - startOfToday) / (24 * 60 * 60 * 1000))
  if (days <= 1) {
    return i18n.t('journey_available_tomorrow') || 'Available tomorrow'
  }
  return i18n.t('journey_available_in_days', { count: days }) || `Available in ${days} days`
}

// NextSessionCard previews the upcoming chapter of the growth journey — which deep
// session comes next and when it unlocks — so the path ahead is always visible.
// When the session is already available it carries a "Start now" button; while
// gated it shows the remaining wait instead.
export const NextSessionCard = memo(function NextSessionCard({ nextSession, onStart, blurTargetRef }: NextSessionCardProps) {
  const isAvailable = new Date(nextSession.availableAt).getTime() <= Date.now()
  const scale = useSharedValue(1)

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }))

  const handlePressIn = useCallback(() => {
    if (isAvailable) {
      scale.value = withSpring(0.97, { damping: 15, stiffness: 400 })
    }
  }, [scale, isAvailable])

  const handlePressOut = useCallback(() => {
    scale.value = withSpring(1, { damping: 15, stiffness: 400 })
  }, [scale])

  const handleStart = useCallback(() => {
    haptic.medium()
    onStart?.(nextSession.session)
  }, [onStart, nextSession.session])

  const details = getSessionDetails(nextSession.session)
  if (!details) return null

  return (
    <GlassCard
      variant="light"
      blurTarget={blurTargetRef}
      padding={0}
      borderRadius={12}
      style={styles.card}
    >
      <XStack padding="$4" gap="$3" alignItems="center">
        <YStack flex={1} gap="$1">
          <Text color="$onGlassSecondary" fontSize={11} fontWeight="600" textTransform="uppercase" letterSpacing={0.5}>
            {(i18n.t('journey_next_session') || 'Next Session')}
          </Text>
          <Text color="$onGlass" fontSize={17} fontWeight="700">
            {details.title}
          </Text>
          {!isAvailable ? (
            <Text color="$onGlassTertiary" fontSize={12}>
              {availabilityText(nextSession.availableAt)}
            </Text>
          ) : null}
        </YStack>
        {isAvailable ? (
          <Pressable
            onPress={handleStart}
            onPressIn={handlePressIn}
            onPressOut={handlePressOut}
          >
            <Reanimated.View style={animatedStyle}>
              <XStack gap="$2" alignItems="center" style={styles.startButton}>
                <Text color="white" fontSize={13} fontWeight="600">
                  {(i18n.t('journey_start_now') || 'Start now')}
                </Text>
                <Mic size={14} color="#fff" />
              </XStack>
            </Reanimated.View>
          </Pressable>
        ) : (
          <Lock size={18} color={INK.secondary} />
        )}
      </XStack>
    </GlassCard>
  )
})

const styles = StyleSheet.create({
  card: {
    overflow: 'hidden',
  },
  startButton: {
    // Dark ink pill so the white CTA label keeps ~7.7:1 on the light glass.
    backgroundColor: INK.primary,
    borderRadius: 99,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
})
