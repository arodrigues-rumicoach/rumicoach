import { useCallback, memo } from 'react'
import { Pressable, View, StyleSheet } from 'react-native'
import { YStack, Text } from 'tamagui'
import { ChevronRight } from 'lucide-react-native'
import Reanimated, { useSharedValue, useAnimatedStyle, withSpring } from 'react-native-reanimated'
import { AnimatedNumber, GlassCard } from '@/components/atoms'
import { haptic } from '@/utils/haptics'
import i18n from '@/i18n'

const CARD_STAGGER_MS = 3 * 120
const CARD_SETTLE_MS = 300
const ITEM_STAGGER_MS = 120

interface ProgressCardProps {
  currentStreak: number
  bestStreak: number
  totalSessions: number
  rumiTimeFormatted: string
  insightsDiscovered: number
  commitmentsKept: number
  onPressStreak: () => void
  onPressSessions: () => void
  onPressInsights: () => void
  onPressCommitments: () => void
}

const StatTile = memo(function StatTile({
  value,
  label,
  onPress,
  accessibilityLabel,
}: {
  value: string
  label: string
  onPress: () => void
  accessibilityLabel: string
}) {
  const scale = useSharedValue(1)

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }))

  const handlePressIn = useCallback(() => {
    // eslint-disable-next-line react-hooks/immutability -- Reanimated shared values are mutable by design
    scale.value = withSpring(0.97, { damping: 15, stiffness: 400 })
    haptic.light()
  }, [scale])

  const handlePressOut = useCallback(() => {
    // eslint-disable-next-line react-hooks/immutability -- Reanimated shared values are mutable by design
    scale.value = withSpring(1, { damping: 15, stiffness: 400 })
  }, [scale])

  return (
    <Pressable
      onPress={onPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      style={{ flex: 1 }}
    >
      <Reanimated.View style={[styles.statTile, animatedStyle]}>
        <YStack flex={1} gap={2}>
          <Text fontSize={24} fontWeight="700" color="$onGlass">
            {value}
          </Text>
          <Text fontSize={11.5} color="$onGlassSecondary">
            {label}
          </Text>
        </YStack>
        <ChevronRight size={16} color="rgba(0,0,0,0.35)" />
      </Reanimated.View>
    </Pressable>
  )
})

export function ProgressCard({
  currentStreak,
  bestStreak,
  totalSessions,
  rumiTimeFormatted,
  insightsDiscovered,
  commitmentsKept,
  onPressStreak,
  onPressSessions,
  onPressInsights,
  onPressCommitments,
}: ProgressCardProps) {
  const base = CARD_STAGGER_MS + CARD_SETTLE_MS

  return (
    <GlassCard variant="light" borderRadius={18} padding={16} gap={12}>
      <Text fontSize={13} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
        {i18n.t('profile_progress_label') || 'Progress'}
      </Text>
      <View style={styles.statsGrid}>
        <View style={styles.statsRow}>
          <AnimatedNumber value={currentStreak} delayMs={base}>
            {(n) => (
              <StatTile
                value={`${n} ${i18n.t('profile_days') || 'days'}`}
                label={(i18n.t('profile_current_streak') || 'current streak') + ` · ${i18n.t('profile_best') || 'best'} ${bestStreak}`}
                onPress={onPressStreak}
                accessibilityLabel={i18n.t('streak_title') || 'Your streak'}
              />
            )}
          </AnimatedNumber>
          <AnimatedNumber value={totalSessions} delayMs={base + ITEM_STAGGER_MS}>
            {(n) => (
              <StatTile
                value={String(n)}
                label={(i18n.t('profile_sessions') || 'sessions') + ` · ${rumiTimeFormatted} ${i18n.t('profile_with_rumi') || 'with Rumi'}`}
                onPress={onPressSessions}
                accessibilityLabel={i18n.t('profile_sessions') || 'sessions'}
              />
            )}
          </AnimatedNumber>
        </View>
        <View style={styles.statsRow}>
          <AnimatedNumber value={insightsDiscovered} delayMs={base + ITEM_STAGGER_MS * 2}>
            {(n) => (
              <StatTile
                value={String(n)}
                label={i18n.t('profile_insights_discovered') || 'insights discovered'}
                onPress={onPressInsights}
                accessibilityLabel={i18n.t('profile_insights_discovered') || 'insights discovered'}
              />
            )}
          </AnimatedNumber>
          <AnimatedNumber value={commitmentsKept} delayMs={base + ITEM_STAGGER_MS * 3}>
            {(n) => (
              <StatTile
                value={String(n)}
                label={i18n.t('profile_commitments_kept') || 'commitments kept'}
                onPress={onPressCommitments}
                accessibilityLabel={i18n.t('profile_commitments_kept') || 'commitments kept'}
              />
            )}
          </AnimatedNumber>
        </View>
      </View>
    </GlassCard>
  )
}

const styles = StyleSheet.create({
  statsGrid: {
    gap: 10,
  },
  statsRow: {
    flexDirection: 'row',
    gap: 10,
  },
  statTile: {
    flex: 1,
    borderRadius: 14,
    padding: 14,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    backgroundColor: 'rgba(0,0,0,0.05)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
  },
})
