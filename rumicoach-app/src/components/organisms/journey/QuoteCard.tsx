import { memo, type RefObject } from 'react'
import { StyleSheet, View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Quote as QuoteIcon } from 'lucide-react-native'
import { DisplayText, GlassCard } from '@/components/atoms'
import { INK } from '@/styles/glass'

interface QuoteCardProps {
  quote: string
  author?: string
  blurTargetRef?: RefObject<View | null>
}

export const QuoteCard = memo(function QuoteCard({ quote, author, blurTargetRef }: QuoteCardProps) {
  return (
    <GlassCard
      variant="light"
      blurTarget={blurTargetRef}
      padding={0}
      borderRadius={16}
      style={styles.card}
    >
      <YStack paddingTop="$6" paddingBottom='$4' paddingHorizontal="$5" gap="$2" position="relative">
        <View style={styles.iconLeft}>
          <QuoteIcon size={24} color={INK.primary} opacity={0.25} />
        </View>
        <View style={styles.iconRight}>
          <QuoteIcon size={42} color={INK.primary} opacity={0.25} />
        </View>
        <DisplayText color="$onGlass" fontSize={22} lineHeight={28} >
          {quote}
        </DisplayText>
        <XStack gap="$2" alignItems="center">
          <YStack height={1} width={24} backgroundColor="$onGlassTertiary" opacity={0.4} />
          <Text color="$onGlassTertiary" fontSize={12} fontWeight="500">
            {author || 'Unknown'}
          </Text>
        </XStack>
      </YStack>
    </GlassCard>
  )
})

const styles = StyleSheet.create({
  card: {
    overflow: 'hidden',
  },
  iconLeft: {
    position: 'absolute',
    top: 4,
    left: 4,
    transform: [{ rotate: '180deg' }],
  },
  iconRight: {
    position: 'absolute',
    top: 4,
    right: 4,
  },
})
