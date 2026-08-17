import { View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { AnimatedBar, GlassCard } from '@/components/atoms'
import { INK } from '@/styles/glass'
import i18n from '@/i18n'

const STREAK_ORANGE = '#f97316'
const STREAK_AMBER = INK.amber
const CARD_STAGGER_MS = 2 * 120
const CARD_SETTLE_MS = 300
const ITEM_STAGGER_MS = 120

interface CurrentStateCardProps {
  wheelEntries: [string, number][]
  focusArea: string | null
  accentColor: string
  blurTarget?: React.RefObject<View | null>
}

export function CurrentStateCard({
  wheelEntries,
  focusArea,
  accentColor,
  blurTarget,
}: CurrentStateCardProps) {
  return (
    <GlassCard variant="light" borderRadius={18} padding={16} gap={12} blurTarget={blurTarget}>
      <XStack justifyContent="space-between" alignItems="baseline">
        <Text fontSize={13} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
          {i18n.t('profile_balance_label') || 'Current State'}
        </Text>
        <Text fontSize={11.5} color="$onGlassTertiary">
          {i18n.t('profile_balance_source') || 'from your Wheel of Life'}
        </Text>
      </XStack>
      {wheelEntries.length > 0 ? (
        <YStack gap={14}>
          {wheelEntries.map(([area, score], index) => {
            const isFocus = focusArea === area
            return (
              <YStack key={area} gap={6}>
                <XStack justifyContent="space-between" alignItems="baseline">
                  <Text fontSize={14} fontWeight="600" color="$onGlass">
                    {area}
                  </Text>
                  <Text
                    fontSize={12.5}
                    fontWeight={isFocus ? '700' : '600'}
                    color={isFocus ? STREAK_AMBER : '$onGlassSecondary'}
                  >
                    {score} / 10{isFocus ? ` · ${i18n.t('profile_focus') || 'focus'}` : ''}
                  </Text>
                </XStack>
                <AnimatedBar
                  targetPercent={Math.min(Math.max(score, 0), 10) * 10}
                  color={isFocus ? STREAK_ORANGE : accentColor}
                  delayMs={CARD_STAGGER_MS + CARD_SETTLE_MS + index * ITEM_STAGGER_MS}
                />
              </YStack>
            )
          })}
        </YStack>
      ) : (
        <Text fontSize={13} lineHeight={20} color="$onGlassSecondary" textAlign="center" paddingVertical={8}>
          {i18n.t('profile_balance_empty') ||
            "Complete the 'Your vision' session with Rumi to see your life balance here."}
        </Text>
      )}
    </GlassCard>
  )
}
