import analytics from '@react-native-firebase/analytics'
import crashlytics from '@react-native-firebase/crashlytics'
import type { FirebaseAdapter } from './types'

const nativeFirebase: FirebaseAdapter = {
  logEvent: async (name, params) => {
    try { await analytics().logEvent(name, params) } catch {}
  },
  setUserId: async (userId) => {
    try { await analytics().setUserId(userId) } catch {}
  },
  trackScreenView: async (screenName, screenClass) => {
    try { await analytics().logScreenView({ screen_name: screenName, screen_class: screenClass || screenName }) } catch {}
  },
  setCrashlyticsUserId: (userId) => {
    try { if (userId) crashlytics().setUserId(userId) } catch {}
  },
  logCrashlyticsError: (error) => {
    try { crashlytics().recordError(error) } catch {}
  },
}

export default nativeFirebase
