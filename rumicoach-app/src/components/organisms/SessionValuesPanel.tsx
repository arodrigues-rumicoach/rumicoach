import { memo } from 'react'
import { XStack, YStack, Text } from 'tamagui'
import i18n from '@/i18n'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

interface SessionValuesPanelProps {
  values: string[]
}

/**
 * The Values session's live reveal: the moment save_top_values fires, the user's
 * chosen values land on screen as pills while Rumi keeps talking — Filipa's guião
 * expects the values to be *displayed*, not just spoken back. Deliberately tiny:
 * a title and the pills, nothing to read while listening.
 */
export const SessionValuesPanel = memo(function SessionValuesPanel({ values }: SessionValuesPanelProps) {
  if (!values || values.length === 0) return null
  return (
    <Reanimated.View
      entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}
      style={{ width: '100%' }}
    >
      <GlassPanel variant="light">
        <YStack alignItems="center" gap="$4" width="100%">
          <Text
            fontSize={12}
            letterSpacing={1.2}
            textTransform="uppercase"
            color="$onGlassSecondary"
          >
            {i18n.t('summary_your_values')}
          </Text>
          <XStack flexWrap="wrap" gap={10} justifyContent="center">
            {values.map((value, index) => (
              <YStack
                key={index}
                paddingHorizontal={16}
                paddingVertical={8}
                borderRadius={999}
                backgroundColor="rgba(16,185,129,0.12)"
                borderWidth={1}
                borderColor="$accentDark"
              >
                <Text fontSize={17} fontWeight="600" color="$onGlass">{value}</Text>
              </YStack>
            ))}
          </XStack>
        </YStack>
      </GlassPanel>
    </Reanimated.View>
  )
})
