import { isWeb } from '../platform'
import nativeRevenueCat from './native'
import webRevenueCat from './web'

export type { RevenueCatAdapter, PurchasesPackage, CustomerInfo, PurchasesOffering, PurchasesOfferings } from './types'

const getRevenueCatAdapter = () => (isWeb ? webRevenueCat : nativeRevenueCat)

export { getRevenueCatAdapter }
