import { memo } from 'react'
import { View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { BlurView } from 'expo-blur'
import { useSettings } from '@/hooks/useSettings'
import { useBlurTarget } from '@/context/BlurContext'
import i18n from '@/i18n'
import type { EisenhowerMatrixData } from '@/api'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

interface EisenhowerGridProps {
  data: EisenhowerMatrixData
}

interface QuadrantConfig {
  key: keyof EisenhowerMatrixData
  accent: string
  label: string
}

const QUADRANTS: QuadrantConfig[] = [
  { key: 'urgent_important', accent: '#ef4444', label: 'eisenhower_urgent_important' },
  { key: 'urgent_not_important', accent: '#f59e0b', label: 'eisenhower_urgent_not_important' },
  { key: 'not_urgent_important', accent: '#3b82f6', label: 'eisenhower_not_urgent_important' },
  { key: 'not_urgent_not_important', accent: '#6b7280', label: 'eisenhower_not_urgent_not_important' },
]

const Quadrant = memo(function Quadrant({ config, tasks, index }: { config: QuadrantConfig; tasks: { task: string; reasoning?: string }[]; index: number }) {
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()

  return (
    <Reanimated.View
      entering={FadeInDown.delay(index * 100).duration(400).springify().damping(20).stiffness(200)}
      style={{ flex: 1 }}
    >
      <BlurView
        blurTarget={blurTargetRef}
        tint="dark"
        intensity={25}
        style={{
          flex: 1,
          borderRadius: 12,
          overflow: 'hidden',
          borderWidth: 1,
          borderColor: 'rgba(255,255,255,0.06)',
          backgroundColor: colorScheme.navBlur,
          minHeight: 100,
        }}
      >
        <YStack padding="$2" gap="$1" flex={1}>
          <View style={{ backgroundColor: config.accent, paddingHorizontal: 6, paddingVertical: 3, borderRadius: 6, alignSelf: 'flex-start' }}>
            <Text color="#fff" fontSize={10} fontWeight="700" textTransform="uppercase">
              {i18n.t(config.label) || config.label}
            </Text>
          </View>
          {(!tasks || tasks.length === 0) ? (
            <Text color="rgba(255,255,255,0.3)" fontSize={11} paddingTop="$1">
              —
            </Text>
          ) : (
            <YStack gap={2} paddingTop="$1">
              {tasks.map((item, idx) => (
                <Text key={idx} color="#fff" fontSize={11} lineHeight={16} numberOfLines={2}>
                  • {item.task}
                </Text>
              ))}
            </YStack>
          )}
        </YStack>
      </BlurView>
    </Reanimated.View>
  )
})

export const EisenhowerGrid = memo(function EisenhowerGrid({ data }: EisenhowerGridProps) {

  if (__DEV__) console.log({ data })
  return (
    <YStack gap={6} width="100%">
      <XStack gap={6} width="100%">
        <Quadrant config={QUADRANTS[0]} tasks={data.urgent_important || []} index={0} />
        <Quadrant config={QUADRANTS[1]} tasks={data.urgent_not_important || []} index={1} />
      </XStack>
      <XStack gap={6} width="100%">
        <Quadrant config={QUADRANTS[2]} tasks={data.not_urgent_important || []} index={2} />
        <Quadrant config={QUADRANTS[3]} tasks={data.not_urgent_not_important || []} index={3} />
      </XStack>
    </YStack>
  )
})
