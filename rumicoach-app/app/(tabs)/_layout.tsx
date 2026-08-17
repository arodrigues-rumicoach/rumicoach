import { useEffect, useRef } from 'react'
import { View, StyleSheet, BackHandler, Platform } from 'react-native'
import { Tabs } from 'expo-router'
import { TopControls , BottomNav } from '@/components/organisms'
import { useI18n } from '../../src/i18n'
import { ScrollNavProvider } from '../../src/context/ScrollNavContext'
import { canGoBack, goBack } from '../../src/utils/tabHistory'
import { WebMaxWidth } from '@/components/templates'
import { isWeb } from '@/adapters/platform'

function TabContent() {
  useI18n()
  const navigationRef = useRef<any>(null)

  useEffect(() => {
    if (Platform.OS === 'web') return
    const backHandler = BackHandler.addEventListener('hardwareBackPress', () => {
      if (canGoBack()) {
        const previousTab = goBack()
        if (previousTab && navigationRef.current) {
          navigationRef.current.navigate(previousTab)
        }
      }
      return true
    })
    return () => backHandler.remove()
  }, [])

  return (
    <ScrollNavProvider>
      <View style={styles.overlay}>
        <TopControls />
        <WebMaxWidth>
          <Tabs
            screenOptions={{
              headerShown: false,
              sceneStyle: { backgroundColor: 'transparent' },
              animation: 'fade',
              animationDuration: 200,
            } as any}
            tabBar={(props) => {
              navigationRef.current = props.navigation
              return <BottomNav {...props} />
            }}
          >
            {/* unmountOnBlur is web-only: on web, inactive screens stay in the
                DOM and show through the transparent backgrounds. On native,
                unmounting forces a full remount on every tab switch, which makes
                every Reanimated entering animation replay — on Android their
                initial values land a frame after first paint, so the content
                flashes fully-rendered before animating. Staying mounted keeps
                switches instant; useFocusEffect refetches keep the data fresh. */}
            <Tabs.Screen name="journey" options={{ title: 'Journey', unmountOnBlur: isWeb } as any} />
            <Tabs.Screen name="session" options={{ title: 'Session', unmountOnBlur: isWeb } as any} />
            <Tabs.Screen name="memories" options={{ title: 'Memories', unmountOnBlur: isWeb } as any} />
            <Tabs.Screen name="profile" options={{ title: 'Profile', unmountOnBlur: isWeb } as any} />
          </Tabs>
        </WebMaxWidth>
      </View>
    </ScrollNavProvider>
  )
}

export default function TabsLayout() {
  return (
    <View style={styles.container}>
      <TabContent />
    </View>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    width: '100%',
  },
  content: {
    flex: 1,
    width: '100%',
  },
  overlay: {
    flex: 1,
    width: '100%',
  },
})
