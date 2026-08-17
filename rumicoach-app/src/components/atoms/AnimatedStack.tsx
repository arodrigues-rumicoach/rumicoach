import { type ReactNode } from 'react'
import Reanimated, {
  FadeInRight,
  FadeInLeft,
  FadeInUp,
  FadeIn,
  FadeOut,
} from 'react-native-reanimated'
import { isWeb } from '@/adapters/platform'

export type StackDirection = 'forward' | 'backward' | 'up'

interface AnimatedStackProps {
  children: ReactNode
  direction?: StackDirection
}

const NATIVE_ENTERING = {
  forward: FadeInRight.duration(320).springify().damping(22).stiffness(180),
  backward: FadeInLeft.duration(320).springify().damping(22).stiffness(180),
  up: FadeInUp.duration(320).springify().damping(22).stiffness(180),
} as const

const WEB_ENTERING = {
  forward: FadeIn.duration(220),
  backward: FadeIn.duration(220),
  up: FadeIn.duration(220),
} as const

const EXITING = FadeOut.duration(180)

/**
 * Direction-aware enter/exit wrapper. Uses Reanimated layout animations
 * (UI-thread, works on iOS, Android, and web via the babel plugin).
 *
 * - `forward` slides in from the right
 * - `backward` slides in from the left
 * - `up` slides up from below
 * - On web, all variants use a snappier fade for a calmer feel
 * - Always exits with a quick fade-out so the previous view unmounts cleanly
 */
export function AnimatedStack({ children, direction = 'up' }: AnimatedStackProps) {
  const entering = isWeb ? WEB_ENTERING[direction] : NATIVE_ENTERING[direction]

  return (
    <Reanimated.View
      entering={entering}
      exiting={EXITING}
      style={{ width: '100%' }}
    >
      {children}
    </Reanimated.View>
  )
}
