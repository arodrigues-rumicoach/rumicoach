import messaging, { type RemoteMessage } from '@react-native-firebase/messaging'
import { PermissionsAndroid, Platform } from 'react-native'
import type { NotificationsAdapter, NotificationHandler, BackgroundMessageHandler } from './types'

const nativeNotifications: NotificationsAdapter = {
  requestPermission: async () => {
    try {
      if (Platform.OS === 'android' && Platform.Version >= 33) {
        const result = await PermissionsAndroid.request(PermissionsAndroid.PERMISSIONS.POST_NOTIFICATIONS)
        return result === PermissionsAndroid.RESULTS.GRANTED
      }
      const authStatus = await messaging().requestPermission()
      return (
        authStatus === messaging.AuthorizationStatus.AUTHORIZED ||
        authStatus === messaging.AuthorizationStatus.PROVISIONAL
      )
    } catch {
      return false
    }
  },
  getToken: async () => {
    try {
      await messaging().registerDeviceForRemoteMessages()
      return await messaging().getToken()
    } catch {
      return null
    }
  },
  registerToken: async (token: string) => {
    try {
      const { api } = await import('../../api/client')
      await api.post('/me/fcm-token', { token, platform: Platform.OS })
    } catch (e) {
      console.error('registerToken error:', e)
    }
  },
  registerBackgroundHandler: (handler: BackgroundMessageHandler) => {
    messaging().setBackgroundMessageHandler(handler as (message: RemoteMessage) => Promise<void>)
  },
  registerForegroundHandler: (handler: NotificationHandler) => {
    return messaging().onMessage(async (remoteMessage) => {
      const title = remoteMessage.notification?.title || 'New Message'
      const body = remoteMessage.notification?.body || ''
      handler(title, body)
    })
  },
}

export default nativeNotifications
