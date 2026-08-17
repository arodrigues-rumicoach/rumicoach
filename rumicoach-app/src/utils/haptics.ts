import { Platform } from 'react-native'
import * as Haptics from 'expo-haptics'
import { isWeb } from '@/adapters/platform'

export const haptic = {
  light: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
  },
  medium: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium)
  },
  heavy: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Heavy)
  },
  selection: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.selectionAsync()
  },
  success: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success)
  },
  error: () => {
    if (isWeb || Platform.OS === 'android') return
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error)
  },
}
