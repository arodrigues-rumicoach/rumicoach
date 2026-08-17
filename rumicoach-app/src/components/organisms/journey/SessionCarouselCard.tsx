import { memo, useCallback, type RefObject } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import { YStack, XStack, Text, isWeb } from 'tamagui'
import { Lock, Mic } from 'lucide-react-native'
import { LinearGradient } from 'expo-linear-gradient'
import { Heading, ThemedButton, GlassCard, LazyImage } from '@/components/atoms'
import { INK } from '@/styles/glass'
import { haptic } from '@/utils/haptics'
import type { SessionType } from '@/api'
import i18n from '@/i18n'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  useAnimatedReaction,
  withSpring,
  withDelay,
  type SharedValue,
} from 'react-native-reanimated'

export interface CarouselItem {
  session: SessionType
  title: string
  subtitle: string
  actionText: string
  imageUrl: string
  isAvailable: boolean
  availableAt?: string
}

// The daily check-in is always open, so an "available now" badge on it would be
// noise. Deep sessions unlock over time, where it's actually news.
const ALWAYS_OPEN: SessionType[] = ['checkin', 'checkin_daily']

interface SessionCarouselCardProps {
  item: CarouselItem
  isActive: boolean | SharedValue<boolean>
  onPress: (session: SessionType) => void
  blurTargetRef?: RefObject<View | null>
  isFirst?: boolean
  isLast?: boolean
}

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

const SPRING = { damping: 18, stiffness: 180 }

export const SessionCarouselCard = memo(function SessionCarouselCard({
  item,
  isActive,
  onPress,
  blurTargetRef,
  isFirst,
  isLast,
}: SessionCarouselCardProps) {
  const scale = useSharedValue(1)

  // Content entrance animations
  const titleOpacity = useSharedValue(0)
  const titleY = useSharedValue(16)
  const subtitleOpacity = useSharedValue(0)
  const subtitleY = useSharedValue(16)
  const buttonOpacity = useSharedValue(0)
  const buttonY = useSharedValue(16)

  // Trigger staggered entrance when card becomes active
  useAnimatedReaction(
    () => {
      if (typeof isActive === 'boolean') return isActive
      return isActive.value
    },
    (current, previous) => {
      if (current && !previous) {
        titleOpacity.value = withDelay(0, withSpring(1, SPRING))
        titleY.value = withDelay(0, withSpring(0, SPRING))
        subtitleOpacity.value = withDelay(80, withSpring(1, SPRING))
        subtitleY.value = withDelay(80, withSpring(0, SPRING))
        buttonOpacity.value = withDelay(160, withSpring(1, SPRING))
        buttonY.value = withDelay(160, withSpring(0, SPRING))
      } else if (!current && previous) {
        titleOpacity.value = withSpring(0, { damping: 20, stiffness: 300 })
        titleY.value = withSpring(12, { damping: 20, stiffness: 300 })
        subtitleOpacity.value = withSpring(0, { damping: 20, stiffness: 300 })
        subtitleY.value = withSpring(12, { damping: 20, stiffness: 300 })
        buttonOpacity.value = withSpring(0, { damping: 20, stiffness: 300 })
        buttonY.value = withSpring(12, { damping: 20, stiffness: 300 })
      }
    },
  )

  const pressStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }))

  const titleStyle = useAnimatedStyle(() => ({
    opacity: titleOpacity.value,
    transform: [{ translateY: titleY.value }],
  }))

  const subtitleStyle = useAnimatedStyle(() => ({
    opacity: subtitleOpacity.value,
    transform: [{ translateY: subtitleY.value }],
  }))

  const buttonStyle = useAnimatedStyle(() => ({
    opacity: buttonOpacity.value,
    transform: [{ translateY: buttonY.value }],
  }))

  const handlePress = useCallback(() => {
    if (item.isAvailable) {
      haptic.medium()
      onPress(item.session)
    }
  }, [item.isAvailable, item.session, onPress])

  return (
    <Reanimated.View style={[styles.root, pressStyle, {
      marginLeft: isFirst && !isWeb ? 16 : 0,
      marginRight: isLast && !isWeb ? 16 : 0,
    }]}>
      <GlassCard
        blurTarget={blurTargetRef}
        padding={0}
        borderRadius={18}
        style={styles.card}
        variant='light'
      >
        <View style={styles.hero}>
          <LazyImage
            source={{ uri: item.imageUrl }}
            style={[
              styles.image,
              !item.isAvailable && styles.imageLocked,
            ]}
            resizeMode="cover"
          />

          <LinearGradient
            colors={['rgba(0,0,0,0)', 'rgba(0,0,0,0.40)', 'rgba(0,0,0,0.72)']}
            locations={[0, 0.45, 1]}
            style={styles.scrim}
          />

          {item.isAvailable && !ALWAYS_OPEN.includes(item.session) && (
            <View style={styles.availableBadge}>
              <Text color="#fff" fontSize={10} fontWeight="700" textTransform="uppercase" letterSpacing={0.5}>
                {i18n.t('journey_available_now') || 'Available now'}
              </Text>
            </View>
          )}

          <YStack flex={1} zIndex={1} justifyContent="flex-end" padding="$4" gap="$3">
            <Reanimated.View style={titleStyle}>
              <Heading color="#fff" fontWeight="bold" fontSize={20} lineHeight={26}>
                {item.title}
              </Heading>
            </Reanimated.View>

            <Reanimated.View style={subtitleStyle}>
              <Text color="rgba(255,255,255,0.85)" fontSize={14} lineHeight={19} numberOfLines={2}>
                {item.subtitle}
              </Text>
            </Reanimated.View>

            <Reanimated.View style={buttonStyle}>
              {item.isAvailable ? (
                <ThemedButton
                  variant="glass"
                  icon={<Mic size={20} />}
                  onPress={handlePress}
                >
                  {item.actionText}
                </ThemedButton>
              ) : (
                <XStack gap="$2" alignItems="center">
                  <Lock size={16} color="rgba(255,255,255,0.7)" />
                  <Text color="rgba(255,255,255,0.7)" fontSize={13} fontWeight="600">
                    {item.availableAt ? availabilityText(item.availableAt) : item.actionText}
                  </Text>
                </XStack>
              )}
            </Reanimated.View>
          </YStack>
        </View>
      </GlassCard>
    </Reanimated.View>
  )
})

const styles = StyleSheet.create({
  root: {
    overflow: 'hidden',
    borderRadius: 18,
  },
  card: {
    overflow: 'hidden',
    borderColor: 'transparent'
  },
  hero: {
    minHeight: 220,
    justifyContent: 'flex-end',
    overflow: 'hidden',
  },
  image: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    opacity: 0.9,
  },
  imageLocked: {
    opacity: 0.7,
  },
  scrim: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
  },
  availableBadge: {
    position: 'absolute',
    top: 12,
    right: 12,
    backgroundColor: INK.primary,
    borderRadius: 99,
    paddingHorizontal: 10,
    paddingVertical: 4,
    zIndex: 2,
  },
})
