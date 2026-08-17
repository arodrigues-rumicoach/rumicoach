import { Platform } from 'react-native'
import Purchases from 'react-native-purchases'
import type { RevenueCatAdapter } from './types'

const apiKeyIOS = process.env.EXPO_PUBLIC_REVENUECAT_API_KEY_IOS || ''
const apiKeyAndroid = process.env.EXPO_PUBLIC_REVENUECAT_API_KEY_ANDROID || ''

const nativeRevenueCat: RevenueCatAdapter = {
  configure(appUserID: string) {
    const apiKey = Platform.OS === 'ios' ? apiKeyIOS : apiKeyAndroid
    if (!apiKey) {
      if (__DEV__) console.warn('[RevenueCat] No API key configured for', Platform.OS)
      return
    }
    Purchases.configure({ apiKey, appUserID })
  },

  async getOfferings() {
    return Purchases.getOfferings()
  },

  async purchasePackage(aPackage) {
    return Purchases.purchasePackage(aPackage)
  },

  async restorePurchases() {
    return Purchases.restorePurchases()
  },

  async getCustomerInfo() {
    return Purchases.getCustomerInfo()
  },

  async logIn(appUserID: string) {
    await Purchases.logIn(appUserID)
  },

  async logOut() {
    await Purchases.logOut()
  },

  addCustomerInfoUpdateListener(listener) {
    const wrappedListener = (customerInfo: any) => {
      listener(customerInfo)
    }
    Purchases.addCustomerInfoUpdateListener(wrappedListener)
    return () => Purchases.removeCustomerInfoUpdateListener(wrappedListener)
  },
}

export default nativeRevenueCat
