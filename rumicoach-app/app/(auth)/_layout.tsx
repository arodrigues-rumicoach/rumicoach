import { useEffect } from 'react'
import { View, StyleSheet } from 'react-native'
import { Stack } from 'expo-router'
import { TopControls } from '@/components/organisms'
import { useI18n } from '../../src/i18n'
import { getAuthAdapter } from '../../src/adapters/auth'
import { WebMaxWidth } from '@/components/templates'
import { stackAnimation } from '@/adapters/platform'

function AuthContent() {
  useI18n()
  useEffect(() => {
    getAuthAdapter().configure()
  }, [])
  return (
    <>
      <TopControls hideSettingsButton showLanguageSwitcher />
      <View style={styles.content}>
        <WebMaxWidth>
          <Stack
            screenOptions={{
              headerShown: false,
              ...stackAnimation(),
              contentStyle: { backgroundColor: 'transparent' },
            }}
          >
            <Stack.Screen name="signin" />
            <Stack.Screen name="signup" />
          </Stack>
        </WebMaxWidth>
      </View>
    </>
  )
}

export default function AuthLayout() {
  return (
    <View style={styles.container}>
      <AuthContent />
    </View>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  content: {
    flex: 1,
    paddingTop: 100,
  },
})
