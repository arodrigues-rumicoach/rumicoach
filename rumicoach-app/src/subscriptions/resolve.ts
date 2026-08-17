// Runtime resolution of catalog product IDs to the app record this build talks to.
//
// Split out from catalog.ts so the catalog stays importable outside Expo — the App Store
// Connect sync script reads it from plain bun, where expo-constants is unavailable.

import Constants from 'expo-constants'

import {
  PROD_BUNDLE_ID,
  SUBSCRIPTION_PRODUCTS,
  TOP_UP_PRODUCTS,
  type BillingPeriod,
  type TopUpKey,
} from './catalog'

/**
 * True when this build talks to the production App Store app record. Derived from the bundle
 * identifier app.config.js already sets per APP_ENV, so a QA build can never accidentally
 * query production product IDs.
 */
export const isProductionBuild = (): boolean =>
  Constants.expoConfig?.ios?.bundleIdentifier === PROD_BUNDLE_ID

const resolveId = (product: { productId: string; qaProductId: string }): string =>
  isProductionBuild() ? product.productId : product.qaProductId

/** The subscription product ID to hand to StoreKit for this build. */
export const productIdFor = (plan: BillingPeriod): string => {
  const product = SUBSCRIPTION_PRODUCTS.find(p => p.plan === plan)
  if (!product) throw new Error(`Unknown subscription plan: ${plan}`)
  return resolveId(product)
}

/** The top-up product ID to hand to StoreKit for this build. */
export const topUpIdFor = (key: TopUpKey): string => {
  const product = TOP_UP_PRODUCTS.find(p => p.key === key)
  if (!product) throw new Error(`Unknown top-up: ${key}`)
  return resolveId(product)
}

/** Every product ID to load at paywall mount, for this build's app record. */
export const allProductIds = (): string[] => [
  ...SUBSCRIPTION_PRODUCTS.map(resolveId),
  ...TOP_UP_PRODUCTS.map(resolveId),
]
