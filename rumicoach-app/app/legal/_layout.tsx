import { Stack } from 'expo-router'
import { WebMaxWidth } from '@/components/templates'

export default function LegalLayout() {
  return (
    <WebMaxWidth>
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Screen name="privacy" />
        <Stack.Screen name="terms" />
      </Stack>
    </WebMaxWidth>
  )
}
