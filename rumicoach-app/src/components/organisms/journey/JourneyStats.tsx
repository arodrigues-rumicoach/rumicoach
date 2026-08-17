import React, { memo, useEffect, useState, type RefObject } from 'react'
import { StyleSheet, View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Flame, Award } from 'lucide-react-native'
import i18n from '@/i18n'
import { Heading, GlassCard } from '@/components/atoms'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  useAnimatedReaction,
  runOnJS,
} from 'react-native-reanimated'

interface JourneyStatsProps {
  streak: number
  badgesEarned: number
  blurTargetRef?: RefObject<View | null>
}

function AnimatedCounter({ value, color }: { value: number; color: string }) {
  const displayValue = useSharedValue(0)
  const [display, setDisplay] = useState(0)

  useEffect(() => {
    displayValue.value = withTiming(value, { duration: 800 })
  }, [value])

  useAnimatedReaction(
    () => displayValue.value,
    (current) => {
      runOnJS(setDisplay)(Math.round(current))
    }
  )

  return <Text color="$color" fontSize={28} fontWeight="bold">{display}</Text>
}

function StatCard({
  value,
  label,
  icon,
  color,
  blurTargetRef,
}: {
  value: number
  label: string
  icon: React.ReactNode
  color: string
  blurTargetRef?: RefObject<View | null>
}) {
  return (
    <GlassCard
      blurTarget={blurTargetRef}
      padding={0}
      borderRadius={12}
      style={[{ flex: 1 }, styles.card]}
    >
      <YStack padding="$4" alignItems="center" gap="$2">
        <YStack backgroundColor={color} style={styles.iconContainer}>
          {icon}
        </YStack>
        <AnimatedCounter value={value} color={color} />
        <Text color="$colorSecondary" fontSize={12}>{label}</Text>
      </YStack>
    </GlassCard>
  )
}

export const JourneyStats = memo(function JourneyStats({ streak, badgesEarned, blurTargetRef }: JourneyStatsProps) {
  return (
    <YStack gap="$3">
      <Heading fontSize={18} fontWeight="600">
        {(i18n.t('journey_your_journey') || 'Your Journey')}
      </Heading>
      <XStack gap="$4">
        <StatCard
          value={streak}
          label={i18n.t('journey_day_streak') || 'Day Streak'}
          icon={<Flame size={22} color="white" fill="white" />}
          color="#f97316"
          blurTargetRef={blurTargetRef}
        />
        <StatCard
          value={badgesEarned}
          label={i18n.t('journey_badges_earned') || 'Badges'}
          icon={<Award size={22} color="white" fill="white" />}
          color="#a855f7"
          blurTargetRef={blurTargetRef}
        />
      </XStack>
    </YStack>
  )
})

const styles = StyleSheet.create({
  card: {
    overflow: 'hidden',
  },
  iconContainer: {
    width: 44,
    height: 44,
    borderRadius: 22,
    alignItems: 'center',
    justifyContent: 'center',
  },
})
