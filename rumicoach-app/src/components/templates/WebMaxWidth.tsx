import { type ReactNode } from 'react'
import { View, StyleSheet } from 'react-native'
import { isWeb } from '@/adapters/platform'
import { WEB_MAX_WIDTH } from '@/utils/constants'

interface WebMaxWidthProps {
  children: ReactNode
  maxWidth?: number
}

/**
 * Constrains content width on web while keeping native layout unchanged.
 * Use inside BlurViews or other full-width containers to cap the inner content
 * at a readable/desktop-friendly width.
 */
export function WebMaxWidth({ children, maxWidth = WEB_MAX_WIDTH }: WebMaxWidthProps) {
  if (!isWeb) {
    return <>{children}</>
  }

  return (
    <View style={[styles.container, { maxWidth }]}>
      {children}
    </View>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    width: '100%',
    alignSelf: 'center',
  },
})
