import type { RevenueCatAdapter } from './types'
import type { PurchasesPackage, CustomerInfo, PurchasesOfferings } from 'react-native-purchases'

const apiKey = process.env.EXPO_PUBLIC_REVENUECAT_API_KEY_WEB || ''

/**
 * The in-flight or settled configure, not the instance itself.
 *
 * Caching the *promise* is what makes this single-flight. The provider calls configure(userId)
 * and getOfferings() back to back without awaiting the first, so with an instance-only cache
 * both calls would find null and both would configure — and the second, having no user id to
 * hand, would configure an anonymous one. Every subsequent purchase would then land on an
 * anonymous customer and the entitlement would never reach the signed-in user.
 */
let instancePromise: Promise<any> | null = null

async function createInstance(appUserId?: string) {
  // Dynamic import to avoid bundling the web SDK on native
  const { Purchases } = await import('@revenuecat/purchases-js')

  if (!apiKey) {
    if (__DEV__) console.warn('[RevenueCat Web] No API key configured')
    return null
  }

  return Purchases.configure({
    apiKey,
    appUserId: appUserId || Purchases.generateRevenueCatAnonymousAppUserId(),
  })
}

function getOrCreateInstance(appUserId?: string) {
  // A failed configure must not be cached, or the app can never recover from one bad start.
  if (!instancePromise) {
    instancePromise = createInstance(appUserId).catch(e => {
      instancePromise = null
      throw e
    })
  }
  return instancePromise
}

/**
 * The configured web SDK, or null when no API key is set.
 *
 * Exported for paywallView.web.tsx. The paywall has to run on the *same* instance as the
 * rest of the adapter: configuring a second one would hand the paywall a different app user
 * id than the purchases it goes on to report, and the entitlement would land on a stranger.
 */
export const getWebPurchases = () => getOrCreateInstance()

const webRevenueCat: RevenueCatAdapter = {
  configure(appUserID: string) {
    // Initialize lazily on first use — web SDK config is async
    getOrCreateInstance(appUserID).catch((e) => {
      if (__DEV__) console.error('[RevenueCat Web] Configure failed:', e)
    })
  },

  async getOfferings(): Promise<PurchasesOfferings> {
    const instance = await getOrCreateInstance()
    if (!instance) return { current: null, all: {} } as PurchasesOfferings
    const offerings = await instance.getOfferings()
    return offerings as PurchasesOfferings
  },

  async purchasePackage(aPackage: PurchasesPackage) {
    const instance = await getOrCreateInstance()
    if (!instance) throw new Error('RevenueCat not configured')
    const result = await instance.purchase({ rcPackage: aPackage })
    return {
      productIdentifier: aPackage.identifier,
      customerInfo: result.customerInfo as CustomerInfo,
    }
  },

  async restorePurchases() {
    const instance = await getOrCreateInstance()
    if (!instance) throw new Error('RevenueCat not configured')
    return (await instance.getCustomerInfo()) as CustomerInfo
  },

  async getCustomerInfo() {
    const instance = await getOrCreateInstance()
    if (!instance) throw new Error('RevenueCat not configured')
    return (await instance.getCustomerInfo()) as CustomerInfo
  },

  // Both of these drop the cached promise before reconfiguring. Reassigning only the
  // instance would leave the old promise in place, and every later caller would keep
  // resolving to the previous user's SDK.

  async logIn(appUserID: string) {
    instancePromise = null
    await getOrCreateInstance(appUserID)
  },

  async logOut() {
    instancePromise = null
    // No id: createInstance mints an anonymous one, which is what a signed-out visitor
    // should be browsing the paywall as.
    await getOrCreateInstance()
  },

  addCustomerInfoUpdateListener(_listener) {
    // Web SDK doesn't have a native listener — return no-op unsubscribe
    return () => true
  },
}

export default webRevenueCat
