import { type ReactNode } from 'react'
import { useIsFocused } from 'expo-router'
import { isWeb } from '@/adapters/platform'
import { ErrorBoundary } from '@/components/organisms/ErrorBoundary'

/**
 * Renders children only when the current tab screen is focused.
 *
 * On web, React Navigation's bottom-tabs keeps screens mounted, which can cause
 * inactive screens to remain in the DOM and show through transparent
 * backgrounds. This wrapper unmounts the screen content when it loses focus.
 *
 * On native, screens stay mounted to preserve scroll position, form state,
 * and animations.
 */
export function TabScreenWrapper({ children }: { children: ReactNode }) {
  const isFocused = useIsFocused()
  if (!isFocused && isWeb) return null
  return <ErrorBoundary>{children}</ErrorBoundary>
}
