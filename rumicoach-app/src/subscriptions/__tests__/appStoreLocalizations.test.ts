import { describe, expect, it } from '@jest/globals'
// ./instance, not the @/i18n barrel — see the note in appStoreLocalizations.ts. Keeping
// both on the same import path means this test exercises what the sync script loads.
import { SUPPORTED_LOCALES } from '@/i18n/instance'
import {
  APP_STORE_LIMITS,
  APP_STORE_LOCALE_BY_APP_LOCALE,
  APP_STORE_LOCALES,
  GROUP_DISPLAY_NAME,
  PLAN_LOCALIZATIONS,
  TOP_UP_LOCALIZATIONS,
  type ProductLocalization,
} from '../appStoreLocalizations'
import {
  ANNUAL_MONTHS_CHARGED,
  SUBSCRIPTION_PRODUCTS,
  TOP_UP_PRODUCTS,
  annualMonthsFree,
  annualPerMonth,
  type BillingPeriod,
  type Currency,
  type TopUpKey,
} from '../catalog'

const PLANS: BillingPeriod[] = ['monthly', 'annual']
const TOP_UPS: TopUpKey[] = ['topup_quick', 'topup_deep', 'topup_power']
const CURRENCIES: Currency[] = ['USD', 'GBP', 'EUR']

// App Store Connect measures these fields in characters and truncates silently
// rather than rejecting. Every string in the catalog is BMP-only, so counting code
// points matches what Apple counts.
const charCount = (value: string) => [...value].length

const expectWithinBudget = (entry: ProductLocalization) => {
  expect(entry.displayName).toBeTruthy()
  expect(entry.description).toBeTruthy()
  expect(entry.displayName.trim()).toBe(entry.displayName)
  expect(entry.description.trim()).toBe(entry.description)
  expect(charCount(entry.displayName)).toBeLessThanOrEqual(APP_STORE_LIMITS.displayName)
  expect(charCount(entry.description)).toBeLessThanOrEqual(APP_STORE_LIMITS.description)
}

describe('App Store locale mapping', () => {
  it('covers every locale the app ships', () => {
    for (const locale of SUPPORTED_LOCALES) {
      expect(APP_STORE_LOCALE_BY_APP_LOCALE[locale]).toBeDefined()
    }
    expect(APP_STORE_LOCALES).toHaveLength(SUPPORTED_LOCALES.length)
  })

  it('maps each app locale to a distinct App Store locale', () => {
    // Two app locales collapsing onto one App Store locale would mean one of them
    // silently loses its store copy.
    expect(new Set(APP_STORE_LOCALES).size).toBe(APP_STORE_LOCALES.length)
  })
})

describe('membership group and plans', () => {
  it('has a group display name within budget for every locale', () => {
    for (const locale of APP_STORE_LOCALES) {
      const name = GROUP_DISPLAY_NAME[locale]
      expect(name).toBeTruthy()
      expect(charCount(name)).toBeLessThanOrEqual(
        APP_STORE_LIMITS.subscriptionGroupDisplayName
      )
    }
  })

  it.each(PLANS)('has complete, in-budget metadata for the %s plan', plan => {
    for (const locale of APP_STORE_LOCALES) {
      expectWithinBudget(PLAN_LOCALIZATIONS[plan][locale])
    }
  })

  it.each(PLANS)('carries no untranslated copy for the %s plan', plan => {
    // A locale left holding the English string is the failure mode this file exists
    // to prevent, so assert it directly for the non-English locales.
    const english = PLAN_LOCALIZATIONS[plan]['en-US']
    for (const locale of APP_STORE_LOCALES.filter(l => !l.startsWith('en-'))) {
      expect(PLAN_LOCALIZATIONS[plan][locale].displayName).not.toBe(english.displayName)
      expect(PLAN_LOCALIZATIONS[plan][locale].description).not.toBe(english.description)
    }
  })

  it('distinguishes monthly from annual in every locale', () => {
    for (const locale of APP_STORE_LOCALES) {
      expect(PLAN_LOCALIZATIONS.monthly[locale].displayName).not.toBe(
        PLAN_LOCALIZATIONS.annual[locale].displayName
      )
      expect(PLAN_LOCALIZATIONS.monthly[locale].description).not.toBe(
        PLAN_LOCALIZATIONS.annual[locale].description
      )
    }
  })
})

describe('minute top-ups', () => {
  it.each(TOP_UPS)('has complete, in-budget metadata for %s', key => {
    for (const locale of APP_STORE_LOCALES) {
      expectWithinBudget(TOP_UP_LOCALIZATIONS[key][locale])
    }
  })

  it('names each tier distinctly in every locale', () => {
    for (const locale of APP_STORE_LOCALES) {
      const names = TOP_UPS.map(key => TOP_UP_LOCALIZATIONS[key][locale].displayName)
      expect(new Set(names).size).toBe(TOP_UPS.length)
    }
  })

  it('mentions its own minute count in every localized description', () => {
    // The minute count is the whole product, so a copy-paste slip between tiers is
    // worth catching mechanically rather than by eye across 60 strings.
    for (const product of TOP_UP_PRODUCTS) {
      for (const locale of APP_STORE_LOCALES) {
        expect(TOP_UP_LOCALIZATIONS[product.key][locale].description).toContain(
          String(product.minutes)
        )
      }
    }
  })
})

describe('subscription catalog', () => {
  it('defines one product per billing period', () => {
    expect(SUBSCRIPTION_PRODUCTS.map(p => p.plan).sort()).toEqual([...PLANS].sort())
  })

  it('keeps every product ID unique across products and environments', () => {
    // Apple scopes product IDs to the account, not the app, so a QA ID colliding
    // with a production ID is unrecoverable — the identifier cannot be reused.
    const ids = [...SUBSCRIPTION_PRODUCTS, ...TOP_UP_PRODUCTS].flatMap(p => [
      p.productId,
      p.qaProductId,
    ])
    expect(new Set(ids).size).toBe(ids.length)
  })

  it.each(CURRENCIES)('charges %s annually for about ten months', currency => {
    // The website prices annual at exactly 10x monthly (399.90). Apple has no .90
    // price point there, so the live price is the nearest one, 399.99 — within a
    // cent per month. Assert the intent holds rather than exact arithmetic, or this
    // test would forbid ever snapping to an available price point again.
    const monthly = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'monthly')!
    const annual = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'annual')!
    const target = monthly.price[currency] * ANNUAL_MONTHS_CHARGED
    expect(Math.abs(annual.price[currency] - target)).toBeLessThan(1)
  })

  it('gives away two months on the annual plan', () => {
    expect(annualMonthsFree()).toBe(2)
  })

  it.each(CURRENCIES)('prices the annual plan below monthly in %s', currency => {
    const monthly = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'monthly')!
    expect(annualPerMonth(currency)).toBeLessThan(monthly.price[currency])
  })

  it('prices larger top-ups at a lower per-minute rate', () => {
    // The tiers only make sense as a ladder; an inversion would make a bigger
    // bundle worse value than a smaller one.
    for (const currency of CURRENCIES) {
      const rates = TOP_UP_PRODUCTS.map(p => p.price[currency] / p.minutes)
      const descending = [...rates].sort((a, b) => b - a)
      expect(rates).toEqual(descending)
    }
  })
})
