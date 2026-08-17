import { isWeb } from '../platform'
import nativeStorage from './native'
import webStorage from './web'

export type { StorageAdapter } from './types'

const getStorageAdapter = () => (isWeb ? webStorage : nativeStorage)

export { getStorageAdapter }
export { default } from './native'
