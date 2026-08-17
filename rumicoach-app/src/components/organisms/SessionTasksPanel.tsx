import { XStack, YStack, Text } from 'tamagui'
import { CheckCircle2, Circle } from 'lucide-react-native'
import i18n from '@/i18n'
import type { SessionTaskItem } from '@/context/SessionContext'
import { INK } from '@/styles/glass'
import { FadeSlideIn, Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

interface SessionTasksPanelProps {
  tasks: SessionTaskItem[]
}

/**
 * In-session "interactive board": the commitments Rumi registers during the
 * session (e.g. the Movement session's first step and today's proof), pushed by
 * the backend as a data-bearing `session_tasks_update` message the moment
 * add_task fires. The user never leaves the session — they watch each
 * commitment appear here, like the Wheel of Life panel.
 */
export function SessionTasksPanel({ tasks }: SessionTasksPanelProps) {
  if (!tasks || tasks.length === 0) return null

  return (
    <Reanimated.View entering={FadeInDown.duration(500).springify().damping(20).stiffness(150)}>
      <GlassPanel variant="light" margin={12}>
        <Heading color="$onGlass">{i18n.t('summary_commitments')}</Heading>
        <YStack width="100%" gap={10}>
          {tasks.map((task, index) => (
            <FadeSlideIn key={task.id ?? index}>
              <XStack
                width="100%"
                alignItems="center"
                gap={10}
                borderRadius={16}
                paddingHorizontal={16}
                paddingVertical={14}
                backgroundColor="rgba(0,0,0,0.05)"
              >
                {task.done
                  ? <CheckCircle2 size={20} color="#065F46" />
                  : <Circle size={20} color={INK.tertiary} />}
                <YStack flex={1} gap={2}>
                  <Text fontSize={15} color="$onGlass">{task.title}</Text>
                  {task.date ? (
                    <Text fontSize={12} color="$onGlassSecondary">{task.date}</Text>
                  ) : null}
                </YStack>
              </XStack>
            </FadeSlideIn>
          ))}
        </YStack>
      </GlassPanel>
    </Reanimated.View>
  )
}
