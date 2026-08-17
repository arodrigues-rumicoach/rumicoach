import type { FirebaseAdapter } from './types'

const webFirebase: FirebaseAdapter = {
  logEvent: async () => {},
  setUserId: async () => {},
  trackScreenView: async () => {},
  setCrashlyticsUserId: () => {},
  logCrashlyticsError: (error) => {
     
    console.error('[Web Crashlytics]', error)
  },
}

export default webFirebase
