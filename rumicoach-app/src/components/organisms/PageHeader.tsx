import { ReactNode } from 'react'
import { View, StyleSheet } from 'react-native'
import { isWeb, Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { BackButton } from '@/components/atoms'

interface PageHeaderProps {
  title: string
  canGoBack?: boolean
  onBackPress?: () => void
  rightAction?: ReactNode
}

export function PageHeader({ title, canGoBack = true, onBackPress, rightAction }: PageHeaderProps) {
  const insets = useSafeAreaInsets()

  return (
    <View style={[styles.container, { marginTop: isWeb ? 24 : insets.top }]}>
      <BackButton canGoBack={canGoBack} onPress={onBackPress} style={styles.leftSlot} />
      <View style={styles.titleContainer}>
        <Text style={styles.title} numberOfLines={1}>
          {title}
        </Text>
      </View>
      <View style={styles.rightSlot}>
        {rightAction}
      </View>
    </View>
  )
}

const styles = StyleSheet.create({
  container: {
    height: 56,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  leftSlot: {
    position: 'absolute',
    left: 16,
    top: 10,
    bottom: 0,
    justifyContent: 'center',
    zIndex: 1
  },
  titleContainer: {
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#fff',
    height: '100%',
    paddingHorizontal: 16,
    borderRadius: 16
  },
  title: {
    fontSize: 17,
    fontWeight: '700',
    color: '#262220',
  },
  rightSlot: {
    position: 'absolute',
    right: 16,
    top: 10,
    bottom: 0,
    justifyContent: 'center',
    alignItems: 'center',
  },
})
