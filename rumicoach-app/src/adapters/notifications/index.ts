import { isWeb } from '../platform'
import webNotifications from './web'

import type { NotificationsAdapter } from './types'

export type { NotificationHandler, BackgroundMessageHandler, NotificationsAdapter } from './types'

let nativeNotifications: NotificationsAdapter | null = null

export const getNotificationsAdapter = (): NotificationsAdapter => {
  if (isWeb) return webNotifications
  if (!nativeNotifications) {
    nativeNotifications = require('./native').default
  }
  return nativeNotifications!
}
