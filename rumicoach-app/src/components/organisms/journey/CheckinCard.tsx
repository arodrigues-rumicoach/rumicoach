import { memo, useCallback } from 'react'
import { StyleSheet, View } from 'react-native'
import { YStack, Text } from 'tamagui'
import { MessageCircle } from 'lucide-react-native'
import i18n from '@/i18n'
import { GlassCard, Heading, ThemedButton } from '@/components/atoms'
import { haptic } from '@/utils/haptics'

interface CheckinCardProps {
  onStart?: () => void
  blurTargetRef?: React.RefObject<View | null>
}

// CheckinCard offers the always-available check-in while the next deep session
// is still locked — the journey continues even between chapters. Copy and CTA
// mirror the Session screen's post-onboarding card (shared session_mind_* keys)
// so "talk to Rumi whenever" reads the same wherever the user meets it.
export const CheckinCard = memo(function CheckinCard({ onStart, blurTargetRef }: CheckinCardProps) {
  const handleStart = useCallback(() => {
    haptic.medium()
    onStart?.()
  }, [onStart])

  return (
    <GlassCard variant="light" blurTarget={blurTargetRef} padding={24} borderRadius={18} style={styles.card}>
      <YStack alignItems="center" gap="$4">
        <Heading color="$onGlass" fontSize={22} fontWeight="600" textAlign="center">
          {(i18n.t('session_mind_title') || "What's on your mind?")}
        </Heading>
        <Text color="$onGlassSecondary" fontSize={14} textAlign="center">
          {(i18n.t('session_mind_subtitle') || "I'm here to talk when you need")}
        </Text>
        <ThemedButton
          variant="solid"
          onPress={handleStart}
          icon={<MessageCircle size={20} color="#fff" />}
          glow
        >
          {(i18n.t('session_talk_to_rumi') || 'Talk to Rumi')}
        </ThemedButton>
      </YStack>
    </GlassCard>
  )
})

const styles = StyleSheet.create({
  card: {
    overflow: 'hidden',
  },
})
