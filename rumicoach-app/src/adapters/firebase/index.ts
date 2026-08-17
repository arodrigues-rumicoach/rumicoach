import { isWeb } from '../platform'
import webFirebase from './web'

import type { FirebaseAdapter } from './types'

export type { FirebaseAdapter } from './types'

let nativeFirebase: FirebaseAdapter | null = null

export const getFirebaseAdapter = (): FirebaseAdapter => {
  if (isWeb) return webFirebase
  if (!nativeFirebase) {
    nativeFirebase = require('./native').default
  }
  return nativeFirebase!
}
