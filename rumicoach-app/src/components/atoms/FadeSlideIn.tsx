import Reanimated, { FadeInDown, FadeInUp, FadeInLeft, FadeInRight } from 'react-native-reanimated'

type SlideDirection = 'up' | 'forward' | 'backward'

interface FadeSlideInProps {
  children: React.ReactNode
  direction?: SlideDirection
}

const getEnteringAnimation = (direction: SlideDirection) => {
  switch (direction) {
    case 'forward':
      return FadeInRight.duration(400).springify().damping(20).stiffness(200)
    case 'backward':
      return FadeInLeft.duration(400).springify().damping(20).stiffness(200)
    case 'up':
    default:
      return FadeInDown.duration(400).springify().damping(20).stiffness(200)
  }
}

export function FadeSlideIn({ children, direction = 'up' }: FadeSlideInProps) {
  return (
    <Reanimated.View
      entering={getEnteringAnimation(direction)}
      style={{ width: '100%' }}
    >
      {children}
    </Reanimated.View>
  )
}
