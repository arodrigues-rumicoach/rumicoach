import { memo, useCallback } from 'react'
import { Pressable, StyleSheet } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Calendar, Trash2 } from 'lucide-react-native'
import { GlassCard } from '@/components/atoms'
import { INK } from '@/styles/glass'
import { useSettings } from '@/hooks/useSettings'
import { haptic } from '@/utils/haptics'
import type { Memory } from '@/api'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated'
import i18n from '@/i18n'

interface MemoryCardProps {
  memory: Memory
  showCategory: boolean
  formatDate: (date: string) => string
  onDelete: (id: string) => void
}

export const MemoryCard = memo(function MemoryCard({ memory, showCategory, formatDate, onDelete }: MemoryCardProps) {
  const { colorScheme } = useSettings()
  const scale = useSharedValue(1)

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }))

  const handlePressIn = useCallback(() => {
    scale.value = withSpring(0.98, { damping: 15, stiffness: 400 })
  }, [scale])

  const handlePressOut = useCallback(() => {
    scale.value = withSpring(1, { damping: 15, stiffness: 400 })
  }, [scale])

  const handleDelete = useCallback(() => {
    haptic.light()
    onDelete(memory.id)
  }, [memory.id, onDelete])

  return (
    <Pressable onPressIn={handlePressIn} onPressOut={handlePressOut}>
      <Reanimated.View style={animatedStyle}>
        <GlassCard variant="light">
          <XStack justifyContent="space-between" alignItems="flex-start">
            <YStack gap="$1" flex={1}>
              <XStack gap="$2" alignItems="center" flexWrap="wrap">
                <Calendar size={12} color={INK.secondary} />
                <Text color="$onGlassTertiary" fontSize={11}>
                  {formatDate(memory.createdAt)}
                </Text>
                {showCategory && (
                  <>
                    <Text color="$onGlassTertiary" fontSize={11}>•</Text>
                    {/* primary, not accent: white 10px label needs ≥4.5 on the chip */}
                    <YStack backgroundColor={colorScheme.primary} paddingHorizontal={6} paddingVertical={1} borderRadius={4}>
                      <Text color="#fff" fontSize={10} fontWeight="600" textTransform="uppercase">
                        {i18n.t(`category_${memory.category}`)}
                      </Text>
                    </YStack>
                  </>
                )}
              </XStack>
            </YStack>
            <Pressable
              onPress={handleDelete}
              hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
              style={styles.deleteButton}
            >
              <Trash2 size={16} color="#B91C1C" />
            </Pressable>
          </XStack>
          <Text color="$onGlass" fontSize={14} lineHeight={20}>
            {memory.content}
          </Text>
        </GlassCard>
      </Reanimated.View>
    </Pressable>
  )
})

const styles = StyleSheet.create({
  deleteButton: {
    padding: 4,
  },
})
