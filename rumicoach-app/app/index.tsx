import { useEffect, useState } from 'react'
import { ActivityIndicator, Platform, StyleSheet, View } from 'react-native'
import { Redirect } from 'expo-router'
import * as ExpoSplashScreen from 'expo-splash-screen'
import { useAuth } from '../src/hooks/useAuth'
import { SplashScreen } from '@/components/organisms'
import { useSettings } from '@/hooks/useSettings'
import { useAudio } from '@/hooks/useAudio'

ExpoSplashScreen.preventAutoHideAsync().catch(() => { });

export default function Index() {
  const { user, isLoading } = useAuth()
  const { colorScheme } = useSettings()
  const { setSplashVisible } = useAudio()
  const [customSplashDone, setCustomSplashDone] = useState(Platform.OS === 'web')

  useEffect(() => {
    if (!isLoading) {
      ExpoSplashScreen.hideAsync().catch(() => { });
    }
  }, [isLoading])

  useEffect(() => {
    setSplashVisible(!customSplashDone)
    return () => {
      setSplashVisible(false)
    }
  }, [customSplashDone, setSplashVisible])

  if (!customSplashDone) {
    return <SplashScreen onHidden={() => setCustomSplashDone(true)} />
  }

  if (isLoading) {
    return (
      <View style={styles.loading}>
        <ActivityIndicator size="large" color={colorScheme.primary} />
      </View>
    )
  }

  if (user) {
    return <Redirect href="/(tabs)/journey" />
  }

  return <Redirect href="/(auth)/signin" />
}

const styles = StyleSheet.create({
  loading: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
})