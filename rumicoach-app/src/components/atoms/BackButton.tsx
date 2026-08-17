import { Pressable, ViewStyle } from 'react-native'
import { ChevronLeft } from 'lucide-react-native'
import { router } from 'expo-router'
import { INK } from '@/styles/glass'

interface BackButtonProps {
  canGoBack?: boolean
  onPress?: () => void
  style?: ViewStyle
}

export function BackButton({ canGoBack = true, onPress, style }: BackButtonProps) {
  if (!canGoBack && !onPress) return null
  return (
    <Pressable onPress={() => (onPress ? onPress() : router.back())} style={[{ borderRadius: 99, overflow: 'hidden', backgroundColor: '#fff', justifyContent: 'center', alignItems: 'center', width: 32, height: 32 }, style]} >
      <ChevronLeft size={24} color={INK.primary} />
    </Pressable>
  )
}
