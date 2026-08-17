import { memo } from 'react'
import { YStack, Text } from 'tamagui'
import i18n from '@/i18n'
import type { ActionPlanData } from '@/context/SessionContext'
import { Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

interface ActionPlanPanelProps {
  data: ActionPlanData
}

export const ActionPlanPanel = memo(function ActionPlanPanel({ data }: ActionPlanPanelProps) {
  return (
    <Reanimated.View entering={FadeInDown.duration(500).springify().damping(20).stiffness(150)}>
      <GlassPanel variant="light">
        <Heading color="$onGlass">{(i18n.t('action_plan') || 'Action Plan')}{data.area ? `: ${data.area}` : ''}</Heading>
        {(data.actions || []).map((task, index) => (
          <YStack key={index} paddingVertical="$2" borderBottomWidth={1} borderColor="rgba(0,0,0,0.10)">
            <Text fontWeight="600" color="$onGlass">{task.title}</Text>
            {task.type === 'recurring' && task.days && task.days.length > 0 && (
              <Text color="$onGlassSecondary" fontSize={13}>
                {task.days.join(', ')}
              </Text>
            )}
          </YStack>
        ))}
      </GlassPanel>
    </Reanimated.View>
  )
})
