import type { NotificationsAdapter, NotificationHandler, BackgroundMessageHandler } from './types'

const webNotifications: NotificationsAdapter = {
  requestPermission: async () => {
    if (typeof Notification === 'undefined') return false
    const result = await Notification.requestPermission()
    return result === 'granted'
  },
  getToken: async () => {
    // Expo web push requires VAPID keys configured in app.json.
    return null
  },
  registerToken: async () => {
    // Web push tokens are handled by the service worker / push subscription flow.
  },
  registerBackgroundHandler: () => {
    // Not applicable in standard web push model; service worker handles this.
  },
  registerForegroundHandler: () => {
    return () => {}
  },
}

export default webNotifications
