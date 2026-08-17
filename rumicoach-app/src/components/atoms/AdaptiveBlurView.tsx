import { forwardRef, type ReactNode, type RefObject } from 'react'
import { BlurView, type BlurViewProps } from 'expo-blur'
import { View, type View as RNView } from 'react-native'
import { isWeb } from '@/adapters/platform'

interface AdaptiveBlurViewProps extends BlurViewProps {
  children: ReactNode
  blurTarget?: RefObject<RNView | null>
}

/**
 * Cross-platform BlurView wrapper.
 *
 * On native, it forwards the optional `blurTarget` ref used for iOS blur-back
 * vibrancy. On web, `blurTarget` is omitted because `expo-blur` does not
 * support it and passing a ref object to the DOM can cause warnings.
 */
export const AdaptiveBlurView = forwardRef<View, AdaptiveBlurViewProps>(
  ({ blurTarget, ...props }, ref) => {
    if (isWeb) {
      return <BlurView ref={ref as any} {...props} />
    }

    return <BlurView ref={ref as any} blurTarget={blurTarget} {...props} />
  }
)
