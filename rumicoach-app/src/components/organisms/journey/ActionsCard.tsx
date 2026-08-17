import { memo, useCallback, useRef, useEffect, Fragment } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Check, Calendar } from 'lucide-react-native'
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withDelay,
  withTiming,
  interpolate,
  interpolateColor,
} from 'react-native-reanimated'
import i18n from '@/i18n'
import { AREA_COLORS, getAreaClass } from './constants'
import { GlassCard, Heading } from '@/components/atoms'
import { GLASS, INK } from '@/styles/glass'
import { haptic } from '@/utils/haptics'
import type { Action } from '@/api'

// Deep green/red inks: ≥3:1 component contrast on the light glass material.
const DONE_GREEN = '#065F46'
const OVERDUE_RED = '#B91C1C'

const SPRING_CONFIG = { damping: 14, stiffness: 260 }

interface ActionRowItemProps {
  action: Action
  isCompleted: boolean
  isOverdue: boolean
  meta: string
  onToggle: () => void
}

const ActionRowItem = memo(function ActionRowItem({
  action,
  isCompleted,
  isOverdue,
  meta,
  onToggle,
}: ActionRowItemProps) {
  const checkboxScale = useSharedValue(1)
  const checkScale = useSharedValue(isCompleted ? 1 : 0)
  const flashOpacity = useSharedValue(0)
  const completionProgress = useSharedValue(isCompleted ? 1 : 0)
  const rowScale = useSharedValue(1)
  const prevCompleted = useRef(isCompleted)

  useEffect(() => {
    if (isCompleted && !prevCompleted.current) {
      // Row: subtle tactile squish that settles back to rest.
      rowScale.value = withSpring(0.985, { stiffness: 500, damping: 18 }, () => {
        rowScale.value = withSpring(1, { stiffness: 500, damping: 18 })
      })
      // Checkbox: squish → overshoot → settle.
      checkboxScale.value = withSpring(0.8, { ...SPRING_CONFIG, stiffness: 320 }, () => {
        checkboxScale.value = withSpring(1.15, SPRING_CONFIG, () => {
          checkboxScale.value = withSpring(1, SPRING_CONFIG)
        })
      })
      // Checkmark: pops in after the checkbox begins its bounce.
      checkScale.value = withDelay(60, withSpring(1, { damping: 12, stiffness: 300 }))
      // Completion state: color + native strikethrough, staggered slightly.
      completionProgress.value = withDelay(40, withTiming(1, { duration: 260 }))
      // Flash overlay.
      flashOpacity.value = withTiming(0.18, { duration: 80 }, () => {
        flashOpacity.value = withTiming(0, { duration: 320 })
      })
    }
    if (!isCompleted && prevCompleted.current) {
      rowScale.value = withSpring(1, { stiffness: 500, damping: 18 })
      checkScale.value = 0
      checkboxScale.value = 1
      completionProgress.value = withTiming(0, { duration: 200 })
      flashOpacity.value = 0
    }
    prevCompleted.current = isCompleted
  }, [isCompleted, checkboxScale, checkScale, completionProgress, flashOpacity, rowScale])

  const checkboxAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: checkboxScale.value }],
  }))

  const checkAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: checkScale.value }],
    opacity: interpolate(checkScale.value, [0, 0.5], [0, 1], { extrapolateRight: 'clamp' }),
  }))

  const flashStyle = useAnimatedStyle(() => ({
    opacity: flashOpacity.value,
  }))

  const rowAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: rowScale.value }],
  }))

  const titleAnimatedStyle = useAnimatedStyle(() => ({
    color: interpolateColor(
      completionProgress.value,
      [0, 1],
      ['#262220', '#4A4540']
    ),
  }))

  return (
    <Pressable
      onPress={onToggle}
      style={({ pressed }) => [styles.actionRow, pressed && styles.actionRowPressed]}
      accessibilityRole="checkbox"
      accessibilityState={{ checked: isCompleted }}
      accessibilityLabel={action.title}
    >
      <Animated.View style={[styles.rowContent, rowAnimatedStyle]}>
        <Animated.View style={[styles.flashOverlay, flashStyle]} />
        <XStack gap="$3.5" alignItems="center">
          <Animated.View
            style={[
              styles.checkbox,
              isCompleted && styles.checkboxDone,
              !isCompleted && isOverdue && styles.checkboxOverdue,
              checkboxAnimatedStyle,
            ]}
          >
            {isCompleted ? (
              <Animated.View style={checkAnimatedStyle}>
                <Check size={16} color="#fff" strokeWidth={3} />
              </Animated.View>
            ) : null}
          </Animated.View>
          <YStack flex={1} gap={3}>
            <Animated.Text
              style={[
                styles.titleText,
                titleAnimatedStyle,
                { textDecorationLine: isCompleted ? 'line-through' : 'none' },
              ]}
            >
              {action.title}
            </Animated.Text>
            <Text color="$onGlassSecondary" fontSize={13} lineHeight={17}>
              {isOverdue && !isCompleted
                ? `${i18n.t('journey_overdue') || 'Overdue'} · ${meta}`
                : meta}
            </Text>
          </YStack>
        </XStack>
      </Animated.View>
    </Pressable>
  )
})

interface ActionsCardProps {
  actions?: Action[]
  focusArea?: string | null
  onToggleAction?: (actionId: string, status: string) => void
  getWeekdayNames?: (days?: number[]) => string
  formatDate?: (dateString?: string) => string
  blurTargetRef?: React.RefObject<View | null>
}

export const ActionsCard = memo(function ActionsCard({
  actions = [], focusArea, onToggleAction, getWeekdayNames, formatDate, blurTargetRef
}: ActionsCardProps) {
  const areaKey = getAreaClass(focusArea || '')
  const areaColor = AREA_COLORS[areaKey] || '$onGlassSecondary'

  const handleToggleAction = useCallback((action: Action) => {
    haptic.medium()
    onToggleAction?.(action.id, action.status)
  }, [onToggleAction])

  const handleEmptyPress = useCallback(() => {
    haptic.heavy()
  }, [])

  if (actions.length === 0) {
    return (
      <GlassCard
        variant="light"
        blurTarget={blurTargetRef}
        padding={0}
        borderRadius={14}
      >
        <Pressable onPress={handleEmptyPress} style={styles.emptyPressable}>
          <YStack gap="$4" alignItems="center" padding="$4">
            <Calendar size={24} color={INK.secondary} />
            <YStack gap="$1" alignItems="center">
              <Text color="$onGlassSecondary" fontSize={13} textAlign='center'>
                {(i18n.t('journey_no_goals') || 'No actions set for today.')}
              </Text>
              <Text color="$onGlassSecondary" fontSize={12} textAlign='center'>
                {(i18n.t('journey_no_goals_subtitle') || 'No actions set for today.')}
              </Text>
              <Text color="$onGlassSecondary" fontSize={12} textAlign='center'>
                {(i18n.t('journey_no_goals_talk_to_rumi') || 'No actions set for today.')}
              </Text>
            </YStack>
          </YStack>
        </Pressable>
      </GlassCard>
    )
  }

  return (
    // One shared glass panel, iOS grouped-list style: the header and every
    // commitment row read through the same material, separated by hairlines.
    // The header lives on the glass because nothing may float on the video.
    <GlassCard variant="light" padding={0} borderRadius={GLASS.radius.card} blurTarget={blurTargetRef} containerView={false}>
      <XStack justifyContent="space-between" alignItems="center" gap="$2" paddingHorizontal={16} paddingTop={16} paddingBottom={8}>
        <Heading color="$onGlass" fontSize={22} fontWeight="700">
          {(i18n.t('journey_current_commitments') || 'Commitments')}
        </Heading>
        {focusArea ? (
          <YStack backgroundColor={areaColor} paddingHorizontal={10} paddingVertical={3} borderRadius={99}>
            <Text color="white" fontSize={12} fontWeight="600">
              {focusArea}
            </Text>
          </YStack>
        ) : null}
      </XStack>

      <YStack>
        {actions.map((action: Action, index: number) => {
          const isCompleted = action.status === 'completed'
          const isOverdue = action.status === 'overdue'
          const meta = action.type === 'recurring'
            ? (i18n.t('journey_recurring_task') || 'Recurring') +
            (action.days && action.days.length > 0 ? ` · ${getWeekdayNames?.(action.days)}` : '')
            : (i18n.t('journey_one_time_task') || 'One-time') +
            (action.date ? ` · ${formatDate?.(action.date)}` : '')

          return (
            <Fragment key={action.id}>
              {index > 0 ? <View style={styles.separator} /> : null}
              <ActionRowItem
                action={action}
                isCompleted={isCompleted}
                isOverdue={isOverdue}
                meta={meta}
                onToggle={() => handleToggleAction(action)}
              />
            </Fragment>
          )
        })}
      </YStack>
    </GlassCard>
  )
})

const styles = StyleSheet.create({
  emptyPressable: {
    width: '100%',
  },
  separator: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: GLASS.separator,
    marginLeft: 60, // aligns with the text column, past the checkbox
  },
  actionRow: {
    paddingVertical: 14,
    paddingHorizontal: 16,
  },
  actionRowPressed: {
    backgroundColor: GLASS.pressedFill,
  },
  rowContent: {
    position: 'relative',
  },
  checkbox: {
    width: 28,
    height: 28,
    borderRadius: 14,
    borderWidth: 2,
    // Dark ink border: ≥3:1 against the light glass (WCAG 1.4.11).
    borderColor: INK.secondary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  checkboxDone: {
    backgroundColor: DONE_GREEN,
    borderColor: DONE_GREEN,
  },
  checkboxOverdue: {
    borderColor: OVERDUE_RED,
  },
  flashOverlay: {
    ...StyleSheet.absoluteFill,
    backgroundColor: DONE_GREEN,
    borderRadius: 0,
  },
  titleText: {
    fontSize: 16,
    lineHeight: 21,
    // Native line-through thickness is controlled by the font/OS; keeps it crisp on all platforms.
    textDecorationColor: INK.secondary,
  },
})
