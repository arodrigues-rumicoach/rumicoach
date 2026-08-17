import { type ReactNode } from 'react'
import { KeyboardAvoidingView, Platform, StyleSheet, View, ScrollView } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { useScrollNav } from '@/context/ScrollNavContext'
import { isWeb, isIOS } from '@/adapters/platform'
import { ErrorBoundary } from '@/components/organisms/ErrorBoundary'
import { useSession } from '@/hooks/useSession'

interface ContentLayoutProps {
  scrollable?: boolean
  centered?: boolean
  keyboardAvoiding?: boolean
  showsVerticalScrollIndicator?: boolean
  showsHorizontalScrollIndicator?: boolean
  children: ReactNode
}

const TOP_OFFSET = isWeb ? 80 : isIOS ? 100 : 80;

const BOTTOM_OFFSET = 80

export function ContentLayout({ scrollable = true, centered = false, keyboardAvoiding = false, showsVerticalScrollIndicator = true, showsHorizontalScrollIndicator = true, children }: ContentLayoutProps) {
  const insets = useSafeAreaInsets()
  const { onScroll } = useScrollNav()
  const { status } = useSession()

  const isConnected = status !== 'disconnected';
  const topOffset = isConnected ? TOP_OFFSET + (isWeb ? 20 : 40) : TOP_OFFSET;
  const bottomPadding = BOTTOM_OFFSET + insets.bottom;

  const content = <ErrorBoundary>{children}</ErrorBoundary>

  if (scrollable) {
    const scrollView = (
      <ScrollView
        keyboardShouldPersistTaps="handled"
        onScroll={onScroll}
        scrollEventThrottle={16}
        showsVerticalScrollIndicator={showsVerticalScrollIndicator}
        showsHorizontalScrollIndicator={showsHorizontalScrollIndicator}
        contentContainerStyle={[
          styles.scrollContent,
          { paddingTop: topOffset, paddingBottom: bottomPadding },
        ]}
      >
        {content}
      </ScrollView>
    )

    if (keyboardAvoiding) {
      return (
        <KeyboardAvoidingView
          style={styles.flex}
          behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
        >
          {scrollView}
        </KeyboardAvoidingView>
      )
    }

    return scrollView
  }

  if (centered) {
    return (
      <View style={[styles.centered, { paddingBottom: bottomPadding }]}>
        {content}
      </View>
    )
  }

  return (
    <View style={[styles.default, { paddingTop: topOffset, paddingBottom: bottomPadding }]}>
      {content}
    </View>
  )
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  scrollContent: { flexGrow: 1 },
  centered: { flex: 1, justifyContent: 'center', alignItems: 'center' },
  default: { flex: 1 },
})
