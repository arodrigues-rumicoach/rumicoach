import { getFirebaseAdapter } from '../adapters/firebase'
import { getNotificationsAdapter } from '../adapters/notifications'

export async function getFCMToken(): Promise<string | null> {
  return getNotificationsAdapter().getToken()
}

export async function requestNotificationPermission(): Promise<boolean> {
  return getNotificationsAdapter().requestPermission()
}

export async function logAnalyticsEvent(name: string, params?: Record<string, unknown>) {
  return getFirebaseAdapter().logEvent(name, params)
}

export async function setAnalyticsUserId(userId: string | null) {
  return getFirebaseAdapter().setUserId(userId)
}

export async function trackScreenView(screenName: string, screenClass?: string) {
  return getFirebaseAdapter().trackScreenView(screenName, screenClass)
}

export function setCrashlyticsUserId(userId: string | null) {
  return getFirebaseAdapter().setCrashlyticsUserId(userId)
}

export function logCrashlyticsError(error: Error) {
  return getFirebaseAdapter().logCrashlyticsError(error)
}

export async function registerFCMToken(token: string): Promise<void> {
  return getNotificationsAdapter().registerToken(token)
}

export async function registerBackgroundMessageHandler() {
  getNotificationsAdapter().registerBackgroundHandler(async (remoteMessage) => {
    console.log('Background message:', remoteMessage)
  })
}

export function registerForegroundMessageHandler(
  callback: (title: string, body: string) => void,
) {
  return getNotificationsAdapter().registerForegroundHandler((title, body) => {
    console.log('Foreground message:', title, body)
    callback(title, body)
  })
}
