// The subscription catalog — the single source of truth for what we sell on the
// App Store, mirrored by hand into App Store Connect.
//
// Prices and product names track the website, which is the commercial source of
// truth: rumicoach-website/src/utils/pricing.ts and the pricing block of
// src/website/translations/*.ts. Keep the two in step — a price that differs
// between web and iOS is a support ticket.
//
// The model is one membership, no tiers ("No premium features locked behind higher
// tiers"): 150 guided voice minutes a month, unused minutes rolling over, plus
// one-time top-ups when someone runs out. The annual plan is exactly 10× monthly,
// which is how "2 months free" stays true in every currency.
//
// Product IDs are unique per Apple *account*, not per app, so a QA app record would need
// its own products. Both sets live here; resolve.ts picks between them at runtime.
//
// This module is deliberately free of native imports so build and CI tooling — notably
// scripts/appstore-subscriptions.ts, which runs outside Expo — can read the catalog without
// pulling in expo-constants. Anything needing the running app belongs in resolve.ts.

export const PROD_BUNDLE_ID = 'coach.rumi.app'

/** Auto-renewable plans share one group so users can switch cadence without re-subscribing. */
export const SUBSCRIPTION_GROUP_REFERENCE_NAME = 'Rumi Membership'

/**
 * RevenueCat offering identifiers. A RevenueCat paywall attaches to exactly one offering,
 * and we show two different paywalls — join the membership, or top up when a member runs
 * out of minutes — so the two sets of products live in two offerings.
 *
 * These strings must match the identifiers in the RevenueCat dashboard; they are how the
 * SDK looks an offering up, and RevenueCat does not let an identifier be renamed.
 */
export const MEMBERSHIP_OFFERING_ID = 'default'
export const TOP_UP_OFFERING_ID = 'topups'

/** Guided voice minutes the membership includes each month. */
export const INCLUDED_MINUTES_PER_MONTH = 150

/** The annual plan costs this many months of the monthly plan — the "2 months free" promise. */
export const ANNUAL_MONTHS_CHARGED = 10

/** Billing currency per market, mirroring getPricing() on the website. */
export type Currency = 'USD' | 'GBP' | 'EUR'

export type BillingPeriod = 'monthly' | 'annual'

/** The billing cadence legible from a product/package identifier, for ids the
 *  catalog does not know — web billing and test stores carry their own slugs
 *  ("membership_monthly"). Annual is checked first: "year"/"annual" never
 *  appear in a monthly id, while "month" could appear in a "12-month" annual
 *  one. Null when the id names neither. */
export function billingPeriodFromProductId(id: string): BillingPeriod | null {
  const lowered = id.toLowerCase()
  if (lowered.includes('annual') || lowered.includes('year')) return 'annual'
  if (lowered.includes('month')) return 'monthly'
  return null
}

/** Top-up keys match the website's signup slugs (?plan=topup_quick) so the backend maps 1:1. */
export type TopUpKey = 'topup_quick' | 'topup_deep' | 'topup_power'

export type PriceByCurrency = Readonly<Record<Currency, number>>

export type SubscriptionProduct = {
  readonly plan: BillingPeriod
  /** App Store Connect product ID for the production app record. */
  readonly productId: string
  /** App Store Connect product ID for the QA app record. */
  readonly qaProductId: string
  /** Internal Reference Name in App Store Connect (never shown to users). */
  readonly referenceName: string
  /** Apple's numeric ID for the live product, handy when debugging StoreKit. */
  readonly appleId: string
  readonly duration: '1 Month' | '1 Year'
  /** Price charged per period, per currency zone. */
  readonly price: PriceByCurrency
}

export type TopUpProduct = {
  readonly key: TopUpKey
  readonly productId: string
  readonly qaProductId: string
  readonly referenceName: string
  /** Apple's numeric ID for the live product, handy when debugging StoreKit. */
  readonly appleId: string
  /** Minutes added to the balance. They roll over and never expire. */
  readonly minutes: number
  readonly price: PriceByCurrency
}

export const SUBSCRIPTION_PRODUCTS: readonly SubscriptionProduct[] = [
  {
    plan: 'monthly',
    productId: 'coach.rumi.app.membership.monthly',
    qaProductId: 'coach.rumi.app.qa.membership.monthly',
    referenceName: 'Rumi Membership Monthly',
    appleId: '6799446065',
    duration: '1 Month',
    price: { USD: 39.99, GBP: 34.99, EUR: 39.99 },
  },
  {
    plan: 'annual',
    // The website advertises 399.90, which is exactly 10x monthly. Apple has no
    // .90 price point at this magnitude, so iOS charges the nearest one: 399.99.
    // That makes the annual plan 10.0025 months of monthly rather than exactly 10.
    productId: 'coach.rumi.app.membership.annual',
    qaProductId: 'coach.rumi.app.qa.membership.annual',
    referenceName: 'Rumi Membership Annual',
    appleId: '6799446593',
    duration: '1 Year',
    price: { USD: 399.99, GBP: 349.99, EUR: 399.99 },
  },
]

/** One-time minute top-ups. Consumable IAPs, not part of the subscription group. */
export const TOP_UP_PRODUCTS: readonly TopUpProduct[] = [
  {
    key: 'topup_quick',
    productId: 'coach.rumi.app.topup.quick',
    qaProductId: 'coach.rumi.app.qa.topup.quick',
    referenceName: 'Rumi Top-Up Quick Boost 60min',
    appleId: '6799446981',
    minutes: 60,
    price: { USD: 15.99, GBP: 13.99, EUR: 15.99 },
  },
  {
    key: 'topup_deep',
    productId: 'coach.rumi.app.topup.deep',
    qaProductId: 'coach.rumi.app.qa.topup.deep',
    referenceName: 'Rumi Top-Up Deep Dive 120min',
    appleId: '6799447172',
    minutes: 120,
    price: { USD: 30.99, GBP: 26.99, EUR: 30.99 },
  },
  {
    key: 'topup_power',
    productId: 'coach.rumi.app.topup.power',
    qaProductId: 'coach.rumi.app.qa.topup.power',
    referenceName: 'Rumi Top-Up Power User 240min',
    appleId: '6799447540',
    minutes: 240,
    price: { USD: 57.99, GBP: 50.99, EUR: 57.99 },
  },
]

/**
 * Months the annual plan gives away, derived rather than hardcoded so the "2 months
 * free" badge cannot drift from the prices above.
 */
export const annualMonthsFree = (): number => 12 - ANNUAL_MONTHS_CHARGED

/** Per-month equivalent of the annual plan — the headline price on the website. */
export const annualPerMonth = (currency: Currency): number => {
  const annual = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'annual')
  if (!annual) throw new Error('Subscription catalog is incomplete')
  return annual.price[currency] / 12
}
