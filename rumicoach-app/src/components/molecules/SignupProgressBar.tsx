import { useEffect, useState, Fragment } from 'react'
import { XStack, Text } from 'tamagui'
import { Check } from 'lucide-react-native'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  interpolateColor,
  Easing,
} from 'react-native-reanimated'
import { INK } from '@/styles/glass'

// Map signup steps to progress bar steps
// NAME → step 0, METHOD/VERIFY → step 1, REGION_TERMS → step 2, COACH_PREFERENCE → step 3, PROFILE_DATA → step 4
const STEP_DATA: { key: string; labelKey: string }[] = [
  { key: 'NAME', labelKey: 'step_name' },
  { key: 'METHOD', labelKey: 'step_account' },
  { key: 'REGION_TERMS', labelKey: 'step_region' },
  { key: 'COACH_PREFERENCE', labelKey: 'step_coach' },
  { key: 'PROFILE_DATA', labelKey: 'step_profile' },
]

const STEP_KEYS = STEP_DATA.map(s => s.key)

function getProgressIndex(step: string): number {
  // VERIFY maps to same progress position as METHOD
  if (step === 'VERIFY') return 1
  const idx = STEP_KEYS.indexOf(step)
  return idx >= 0 ? idx : 0
}

interface SignupProgressBarProps {
  currentStep: string
}

interface AnimatedStepProps {
  isCompleted: boolean
  isCurrent: boolean
  stepNumber: number
  accent: string
}

function AnimatedStepCircle({ isCompleted, isCurrent, stepNumber, accent }: AnimatedStepProps) {
  const progress = useSharedValue(isCompleted ? 1 : isCurrent ? 0.5 : 0)
  const checkScale = useSharedValue(0)

  useEffect(() => {
    if (isCompleted) {
      progress.value = withSpring(1, { damping: 16, stiffness: 200 })
      checkScale.value = withSpring(1, { damping: 12, stiffness: 200 })
    } else if (isCurrent) {
      progress.value = withTiming(0.5, { duration: 200 })
      checkScale.value = withTiming(0, { duration: 100 })
    } else {
      progress.value = withTiming(0, { duration: 200 })
      checkScale.value = withTiming(0, { duration: 100 })
    }
  }, [isCompleted, isCurrent, progress, checkScale])

  const animatedStyle = useAnimatedStyle(() => {
    const backgroundColor = interpolateColor(
      progress.value,
      [0, 0.5, 1],
      ['rgba(0,0,0,0.10)', `${accent}40`, accent]
    )
    return {
      backgroundColor,
      transform: [{ scale: 0.92 + progress.value * 0.08 }],
    }
  })

  const checkAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: checkScale.value }],
    opacity: checkScale.value,
  }))

  return (
    <Reanimated.View
      style={[
        {
          width: 28,
          height: 28,
          borderRadius: 14,
          alignItems: 'center',
          justifyContent: 'center',
          borderWidth: isCurrent ? 2 : 0,
          borderColor: `${accent}66`,
        },
        animatedStyle,
      ]}
    >
      {isCompleted ? (
        <Reanimated.View style={checkAnimatedStyle}>
          <Check size={14} color="white" strokeWidth={3} />
        </Reanimated.View>
      ) : (
        <Text
          fontSize={12}
          fontWeight="700"
          color={INK.primary}
        >
          {stepNumber}
        </Text>
      )}
    </Reanimated.View>
  )
}

function AnimatedConnector({ active, accent }: { active: boolean; accent: string }) {
  const colorProgress = useSharedValue(active ? 1 : 0)
  const widthProgress = useSharedValue(active ? 1 : 0)

  useEffect(() => {
    colorProgress.value = withTiming(active ? 1 : 0, { duration: 380, easing: Easing.out(Easing.cubic) })
    widthProgress.value = withSpring(active ? 1 : 0, { damping: 20, stiffness: 200 })
  }, [active, colorProgress, widthProgress])

  const animatedStyle = useAnimatedStyle(() => ({
    backgroundColor: interpolateColor(
      colorProgress.value,
      [0, 1],
      ['rgba(0,0,0,0.10)', accent]
    ),
    transform: [{ scaleX: 0.3 + widthProgress.value * 0.7 }],
  }))

  return (
    <Reanimated.View
      style={[
        {
          flex: 1,
          height: 2,
          borderRadius: 1,
          marginHorizontal: 6,
          transformOrigin: 'left',
        },
        animatedStyle,
      ]}
    />
  )
}

export function SignupProgressBar({ currentStep }: SignupProgressBarProps) {
  const currentIndex = getProgressIndex(currentStep)

  return (
    <XStack alignItems="center" justifyContent="center" paddingHorizontal={8} width='100%'>
      {STEP_DATA.map((item, index) => {
        const isCompleted = index < currentIndex
        const isCurrent = index === currentIndex
        const isLast = index === STEP_DATA.length - 1

        return (
          <Fragment key={item.key}>
            <AnimatedStepCircle
              isCompleted={isCompleted}
              isCurrent={isCurrent}
              stepNumber={index + 1}
              accent="#10b981"
            />

            {!isLast && (
              <AnimatedConnector active={index < currentIndex} accent="#10b981" />
            )}
          </Fragment>
        )
      })}
    </XStack>
  )
}
