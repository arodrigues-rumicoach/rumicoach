#!/usr/bin/env bun
/**
 * Push the subscription catalog to Google Play.
 *
 * Mirror of scripts/appstore-subscriptions.ts for the Play Developer API. Same source of
 * truth (src/subscriptions/), same dry-run-by-default contract.
 *
 *   bun scripts/playstore-products.ts inspect            # what exists now
 *   bun scripts/playstore-products.ts sync               # print the plan, change nothing
 *   bun scripts/playstore-products.ts sync --apply       # execute the plan
 *
 * Credentials come from a Google Cloud service account with Play Developer API access,
 * linked under Play Console → Users and permissions:
 *
 *   PLAY_SERVICE_ACCOUNT_JSON=~/.playconsole/rumi-play-sa.json
 *
 * Keep the JSON outside the repo.
 *
 * ── Play models subscriptions differently from Apple ──────────────────────────────
 * Apple wants two separate products; Play wants ONE subscription carrying two base plans.
 * So `coach.rumi.app.membership` has base plans `monthly` and `annual`, and RevenueCat maps
 * its packages to the Apple product on iOS and the Play base plan on Android.
 *
 * ── On the annual price ───────────────────────────────────────────────────────────
 * Prices come from catalog.ts, which holds 399.99 — the nearest point Apple offers to the
 * website's 399.90. Play accepts arbitrary amounts and *could* carry the exact 399.90, but
 * reading the shared catalog keeps iOS and Android identical, which is worth more than nine
 * cents of fidelity to the website. Change catalog.ts if you want them to diverge.
 */

import { createSign } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'

import {
  PROD_BUNDLE_ID,
  SUBSCRIPTION_PRODUCTS,
  TOP_UP_PRODUCTS,
  type Currency,
} from '../src/subscriptions/catalog'
import {
  APP_STORE_LOCALE_BY_APP_LOCALE,
  GROUP_DISPLAY_NAME,
  PLAN_LOCALIZATIONS,
  TOP_UP_LOCALIZATIONS,
} from '../src/subscriptions/appStoreLocalizations'
// ./instance, not the i18n barrel: the barrel re-exports I18nProvider, which pulls in react
// and expo-localization and drags React Native's Flow-typed source into this plain Bun
// process, where there is no Metro to strip Flow. Same reason as appStoreLocalizations.ts.
import { SUPPORTED_LOCALES, type SupportedLocale } from '../src/i18n/instance'

const API = 'https://androidpublisher.googleapis.com/androidpublisher/v3'
const SCOPE = 'https://www.googleapis.com/auth/androidpublisher'

/** Play's snapshot of the region list that prices are expressed against. */
const REGIONS_VERSION = '2022/02'

/**
 * Starting point only. regionalPrices() overwrites this with whatever version
 * pricing:convertRegionPrices reports, so reads and writes agree on each region's currency.
 */
let resolvedRegionsVersion = REGIONS_VERSION

/** The single Play subscription that carries both base plans. */
const SUBSCRIPTION_ID = 'coach.rumi.app.membership'

/**
 * Play language codes for the locales we ship. Identical to our i18n codes except Norwegian,
 * which Play spells no-NO rather than nb-NO.
 */
const PLAY_LANGUAGE: Record<SupportedLocale, string> = {
  'en-US': 'en-US',
  'en-GB': 'en-GB',
  'pt-BR': 'pt-BR',
  'pt-PT': 'pt-PT',
  'es-ES': 'es-ES',
  'de-DE': 'de-DE',
  'fr-FR': 'fr-FR',
  'it-IT': 'it-IT',
  'ja-JP': 'ja-JP',
  'zh-CN': 'zh-CN',
  'ko-KR': 'ko-KR',
  'hi-IN': 'hi-IN',
  'pl-PL': 'pl-PL',
  'tr-TR': 'tr-TR',
  // Play spells Ukrainian bare, not uk-UA.
  'uk-UA': 'uk',
  'sv-SE': 'sv-SE',
  'nl-NL': 'nl-NL',
  // ...and Norwegian no-NO, not nb-NO.
  'nb-NO': 'no-NO',
  'da-DK': 'da-DK',
  'fi-FI': 'fi-FI',
}

/**
 * Regions where we pin the price by hand. Everywhere else takes Play's own conversion of the
 * USD base, which is the same bargain we accepted on the App Store.
 */
const PINNED: Record<'GBP' | 'EUR', string[]> = {
  GBP: ['GB'],
  EUR: ['PT', 'ES', 'DE', 'FR', 'IT', 'NL', 'FI'],
}

// ── money ───────────────────────────────────────────────────────────────────────

/** Play expresses money as whole units plus nanos; 39.99 → { units: 39, nanos: 990000000 }. */
const money = (amount: number, currencyCode: Currency | string) => {
  const units = Math.floor(amount)
  // Round before scaling: 0.1 + 0.2 style drift would otherwise produce 989999999 nanos.
  const nanos = Math.round((amount - units) * 1e9)
  return { currencyCode, units: String(units), nanos }
}

// ── auth ────────────────────────────────────────────────────────────────────────

const base64url = (input: Buffer | string) =>
  Buffer.from(input).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

type ServiceAccount = { client_email: string; private_key: string }

const loadServiceAccount = (): ServiceAccount => {
  const path = process.env.PLAY_SERVICE_ACCOUNT_JSON
  if (!path) {
    console.error('Missing PLAY_SERVICE_ACCOUNT_JSON. See the header of this file.')
    process.exit(1)
  }
  return JSON.parse(readFileSync(path.replace(/^~/, homedir()), 'utf8'))
}

/**
 * Two ways in, because the org enforces iam.disableServiceAccountKeyCreation and there is no
 * downloadable key to be had:
 *
 *   PLAY_ACCESS_TOKEN  — a ready androidpublisher token. This is the keyless path: CI mints it
 *                        from Bitbucket's OIDC identity via Workload Identity Federation, the
 *                        same exchange scripts/play-upload.sh does. Locally you can produce one
 *                        with `gcloud auth print-access-token --impersonate-service-account=…`.
 *   PLAY_SERVICE_ACCOUNT_JSON — a downloaded key, for accounts where policy still allows them.
 *
 * Service-account OAuth signs an RS256 assertion and trades it for a token. Tokens last an hour;
 * a full sync is well short of that, but we re-mint at 50 minutes so a slow run cannot die
 * halfway through the listings. A supplied PLAY_ACCESS_TOKEN is used as-is and never re-minted,
 * so a very long run on a near-expired token is the one case that can 401 mid-flight.
 */
let accessToken = ''
let tokenMintedAt = 0

const mintToken = async (): Promise<string> => {
  const sa = loadServiceAccount()
  const now = Math.floor(Date.now() / 1000)
  const header = { alg: 'RS256', typ: 'JWT' }
  const claims = {
    iss: sa.client_email,
    scope: SCOPE,
    aud: 'https://oauth2.googleapis.com/token',
    iat: now,
    exp: now + 3600,
  }
  const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(claims))}`
  const signer = createSign('RSA-SHA256')
  signer.update(signingInput)
  const assertion = `${signingInput}.${base64url(signer.sign(sa.private_key))}`

  const res = await fetch('https://oauth2.googleapis.com/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'urn:ietf:params:oauth:grant-type:jwt-bearer',
      assertion,
    }),
  })
  const json = await res.json()
  if (!res.ok) throw new Error(`Token exchange failed: ${JSON.stringify(json)}`)
  return json.access_token
}

const currentToken = async (): Promise<string> => {
  const supplied = process.env.PLAY_ACCESS_TOKEN
  if (supplied) return supplied

  if (!process.env.PLAY_SERVICE_ACCOUNT_JSON) {
    console.error(
      'No credentials. Set PLAY_ACCESS_TOKEN (keyless — see the header) or\n' +
        'PLAY_SERVICE_ACCOUNT_JSON if your org still permits downloaded keys.'
    )
    process.exit(1)
  }

  const now = Date.now()
  if (!accessToken || now - tokenMintedAt > 50 * 60 * 1000) {
    accessToken = await mintToken()
    tokenMintedAt = now
  }
  return accessToken
}

// ── transport ───────────────────────────────────────────────────────────────────

const APPLY = process.argv.includes('--apply')
let writes = 0

type Json = Record<string, any>

/**
 * User credentials (gcloud application-default) carry no billing project, and
 * androidpublisher refuses to serve them without one — "requires a quota project, which is
 * not set by default". The project is passed per request rather than baked into the token,
 * so it has to be a header. Service-account tokens have an implicit project and ignore this.
 */
const QUOTA_PROJECT = process.env.PLAY_QUOTA_PROJECT

const request = async (method: string, path: string, body?: Json): Promise<Json> => {
  const res = await fetch(path.startsWith('http') ? path : `${API}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${await currentToken()}`,
      ...(QUOTA_PROJECT ? { 'x-goog-user-project': QUOTA_PROJECT } : {}),
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
  const text = await res.text()
  // Not every failure is JSON: a wrong path returns Google's HTML 404, and blindly parsing it
  // buried the real status behind "Unrecognized token '<'".
  let json: Json = {}
  try {
    json = text ? JSON.parse(text) : {}
  } catch {
    throw new Error(
      `${method} ${path} → ${res.status} (non-JSON response)\n  ${text.slice(0, 300).replace(/\s+/g, ' ')}`
    )
  }
  if (!res.ok) {
    throw new Error(`${method} ${path} → ${res.status}\n  ${json.error?.message ?? text}`)
  }
  return json
}

/**
 * Play caps prices per region, and a USD amount converted straight across can land above the
 * ceiling — the annual plan converts to ₩620,000 in Korea against a ₩600,000 limit. The
 * ceilings are not published anywhere we can read up front, but the rejection names the exact
 * bound, so clamp the offending region to it and retry rather than discovering them one failed
 * run at a time. Bounded, because a rejection we cannot parse must surface rather than spin.
 */
// The bound arrives with its currency symbol attached ("and ₩600,000"), so skip any non-digit
// prefix before the number. Play prefixes the message with the offending base plan
// ("annual: Price for KR must be…") and that prefix is load-bearing: a subscription body holds
// both plans, and clamping every match would drag the monthly price up to the annual ceiling.
const PRICE_LIMIT = /(?:([\w-]+): )?Price for ([A-Z]{2}) must be between \S+ and \D*([\d.,]+)/

/**
 * Clamp one region's price to Play's maximum. Scoped to a single base plan when the rejection
 * named one, so a limit hit on the annual plan cannot rewrite the monthly plan's price.
 */
const clampRegion = (
  body: Json,
  regionCode: string,
  max: number,
  basePlanId?: string
): boolean => {
  let clamped = false
  const walk = (node: any) => {
    if (!node || typeof node !== 'object') return
    if (Array.isArray(node)) return node.forEach(walk)
    if (node.regionCode === regionCode && node.price?.currencyCode) {
      node.price = money(max, node.price.currencyCode)
      clamped = true
    }
    Object.values(node).forEach(walk)
  }

  if (basePlanId) {
    const plans: any[] = body.basePlans ?? body.oneTimeProduct?.basePlans ?? []
    const plan = plans.find(p => p.basePlanId === basePlanId)
    // Only fall through to the whole body when the name matched nothing — otherwise a
    // mis-scoped clamp silently corrupts a price we were not asked to change.
    if (plan) {
      walk(plan)
      return clamped
    }
  }
  walk(body)
  return clamped
}

const mutate = async (method: string, path: string, body: Json | undefined, label: string) => {
  writes++
  if (!APPLY) {
    console.log(`  [dry-run] ${label}`)
    return {} as Json
  }
  for (let attempt = 0; attempt < 30; attempt++) {
    try {
      const result = await request(method, path, body)
      console.log(`  ✓ ${label}`)
      return result
    } catch (error: any) {
      const hit = PRICE_LIMIT.exec(error.message)
      if (!hit || !body) throw error
      const [, basePlanId, regionCode, rawMax] = hit
      const max = Number(rawMax.replace(/,/g, ''))
      if (!Number.isFinite(max) || !clampRegion(body, regionCode, max, basePlanId)) throw error
      console.log(
        `    ${basePlanId ? `${basePlanId}/` : ''}${regionCode}: above Play's ceiling, clamped to ${max}`
      )
    }
  }
  throw new Error(`${label}: still rejected after 30 price clamps`)
}

// ── pricing ─────────────────────────────────────────────────────────────────────

/**
 * Ask Play to convert a USD amount into every region, then overwrite the regions we pin.
 * Play has no equivalent of Apple's automatic equalization on write — the API wants an
 * explicit price per region — so this endpoint is what keeps us from hand-listing ~175 rows.
 */
const regionalPrices = async (
  usd: number,
  gbp: number,
  eur: number
): Promise<Array<{ regionCode: string; price: Json }>> => {
  const converted = await request(
    'POST',
    `/applications/${PROD_BUNDLE_ID}/pricing:convertRegionPrices`,
    { price: money(usd, 'USD') }
  )
  // Writes must declare the same regions version the conversion used, or the two disagree
  // about which currency a region is on. Bulgaria is the live example: conversion returns EUR
  // (it has adopted the euro) while the hardcoded 2022/02 snapshot still expects BGN, and the
  // write 400s. Taking the version from the response keeps them in lockstep.
  const version = converted.regionVersion?.version
  if (version && version !== resolvedRegionsVersion) {
    resolvedRegionsVersion = version
    console.log(`  regions version from Play: ${version}`)
  }

  const rows: Array<{ regionCode: string; price: Json }> = Object.entries(
    converted.convertedRegionPrices ?? {}
  ).map(([regionCode, v]: [string, any]) => ({ regionCode, price: v.price }))

  const pin = (codes: string[], amount: number, currency: string) => {
    for (const regionCode of codes) {
      const existing = rows.find(r => r.regionCode === regionCode)
      const price = money(amount, currency)
      if (existing) existing.price = price
      else rows.push({ regionCode, price })
    }
  }
  pin(PINNED.GBP, gbp, 'GBP')
  pin(PINNED.EUR, eur, 'EUR')
  // The US is the conversion source, but pin it anyway so rounding cannot drift the anchor.
  pin(['US'], usd, 'USD')
  return rows
}

// ── listings ────────────────────────────────────────────────────────────────────

/**
 * Reuse the store copy already written for the App Store. Play allows 55 chars of title and
 * 80 (subscriptions) / 200 (one-time) of description, comfortably above Apple's 30/45, so
 * anything that fits Apple fits here.
 */
const listingsFor = (
  pick: (appStoreLocale: string) => { displayName: string; description: string },
  benefit?: string
) =>
  SUPPORTED_LOCALES.map(locale => {
    const { displayName, description } = pick(APP_STORE_LOCALE_BY_APP_LOCALE[locale])
    return {
      languageCode: PLAY_LANGUAGE[locale],
      title: displayName,
      description,
      ...(benefit ? { benefits: [benefit] } : {}),
    }
  })

// ── commands ────────────────────────────────────────────────────────────────────

const inspect = async () => {
  const subs = await request('GET', `/applications/${PROD_BUNDLE_ID}/subscriptions`)
  console.log(`Subscriptions: ${subs.subscriptions?.length ?? 0}`)
  for (const s of subs.subscriptions ?? []) {
    console.log(`  - ${s.productId}`)
    for (const bp of s.basePlans ?? []) {
      console.log(`      base plan ${bp.basePlanId} state=${bp.state} regions=${bp.regionalConfigs?.length ?? 0}`)
    }
    console.log(`      listings: ${(s.listings ?? []).map((l: Json) => l.languageCode).join(', ') || 'none'}`)
  }

  const otp = await request('GET', `/applications/${PROD_BUNDLE_ID}/oneTimeProducts`)
  console.log(`\nOne-time products: ${otp.oneTimeProducts?.length ?? 0}`)
  for (const p of otp.oneTimeProducts ?? []) {
    console.log(`  - ${p.productId} listings=${(p.listings ?? []).length}`)
  }
}

const sync = async () => {
  const monthly = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'monthly')!
  const annual = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'annual')!

  console.log('Resolving regional prices (Play converts, then we pin GB and the eurozone)…')
  const monthlyRegions = await regionalPrices(
    monthly.price.USD,
    monthly.price.GBP,
    monthly.price.EUR
  )
  const annualRegions = await regionalPrices(annual.price.USD, annual.price.GBP, annual.price.EUR)
  console.log(`  monthly: ${monthlyRegions.length} regions | annual: ${annualRegions.length} regions`)

  // One subscription, two base plans. Base plans are created in DRAFT and must be activated
  // in the Console (or via basePlans.activate) before they can be sold.
  const subscription = {
    packageName: PROD_BUNDLE_ID,
    productId: SUBSCRIPTION_ID,
    // The listing belongs to the subscription, not the base plan — Play shows one title to
    // monthly and annual subscribers alike. So the title is the membership name rather than
    // either plan's name, and the description is the part true of both: 150 minutes a month.
    listings: listingsFor(loc => ({
      displayName: GROUP_DISPLAY_NAME[loc as keyof typeof GROUP_DISPLAY_NAME],
      description:
        PLAN_LOCALIZATIONS.monthly[loc as keyof typeof PLAN_LOCALIZATIONS.monthly].description,
    })),
    basePlans: [
      {
        basePlanId: 'monthly',
        autoRenewingBasePlanType: {
          billingPeriodDuration: 'P1M',
          gracePeriodDuration: 'P7D',
          resubscribeState: 'RESUBSCRIBE_STATE_ACTIVE',
          prorationMode: 'SUBSCRIPTION_PRORATION_MODE_CHARGE_ON_NEXT_BILLING_DATE',
        },
        regionalConfigs: monthlyRegions.map(r => ({
          regionCode: r.regionCode,
          newSubscriberAvailability: true,
          price: r.price,
        })),
      },
      {
        basePlanId: 'annual',
        autoRenewingBasePlanType: {
          billingPeriodDuration: 'P1Y',
          gracePeriodDuration: 'P7D',
          resubscribeState: 'RESUBSCRIBE_STATE_ACTIVE',
          prorationMode: 'SUBSCRIPTION_PRORATION_MODE_CHARGE_ON_NEXT_BILLING_DATE',
        },
        regionalConfigs: annualRegions.map(r => ({
          regionCode: r.regionCode,
          newSubscriberAvailability: true,
          price: r.price,
        })),
      },
    ],
  }

  console.log('\nSubscription:')
  const existing = await request('GET', `/applications/${PROD_BUNDLE_ID}/subscriptions`).catch(
    () => ({ subscriptions: [] })
  )
  const alreadyThere = (existing.subscriptions ?? []).some(
    (s: Json) => s.productId === SUBSCRIPTION_ID
  )
  if (alreadyThere) {
    await mutate(
      'PATCH',
      `/applications/${PROD_BUNDLE_ID}/subscriptions/${SUBSCRIPTION_ID}` +
        `?updateMask=listings,basePlans&regionsVersion.version=${resolvedRegionsVersion}`,
      subscription,
      `update ${SUBSCRIPTION_ID} (2 base plans, ${subscription.listings.length} listings)`
    )
  } else {
    await mutate(
      'POST',
      `/applications/${PROD_BUNDLE_ID}/subscriptions` +
        `?productId=${SUBSCRIPTION_ID}&regionsVersion.version=${resolvedRegionsVersion}`,
      subscription,
      `create ${SUBSCRIPTION_ID} (2 base plans, ${subscription.listings.length} listings)`
    )
  }

  // Top-ups. oneTimeProducts.patch upserts when allowMissing=true, so this is the same call
  // whether the product exists or not.
  console.log('\nTop-ups:')
  for (const product of TOP_UP_PRODUCTS) {
    const regions = await regionalPrices(
      product.price.USD,
      product.price.GBP,
      product.price.EUR
    )
    const body = {
      packageName: PROD_BUNDLE_ID,
      productId: product.productId,
      listings: listingsFor(
        loc => TOP_UP_LOCALIZATIONS[product.key][loc as keyof (typeof TOP_UP_LOCALIZATIONS)['topup_quick']]
      ),
      purchaseOptions: [
        {
          purchaseOptionId: 'default',
          buyOption: { legacyCompatible: true, multiQuantityEnabled: false },
          regionalPricingAndAvailabilityConfigs: regions.map(r => ({
            regionCode: r.regionCode,
            price: r.price,
            availability: 'AVAILABLE',
          })),
        },
      ],
    }
    // One-time products have no individual PATCH — that URL 404s at the routing layer while
    // GET on the identical path works. Writes go through :batchUpdate. One product per call
    // rather than all three in one batch, so a price clamp only touches the product whose
    // region was rejected; clamping across a shared batch would rewrite the other two.
    await mutate(
      'POST',
      `/applications/${PROD_BUNDLE_ID}/oneTimeProducts:batchUpdate`,
      {
        requests: [
          {
            oneTimeProduct: body,
            updateMask: 'listings,purchaseOptions',
            allowMissing: true,
            regionsVersion: { version: resolvedRegionsVersion },
          },
        ],
      },
      `upsert ${product.productId} (${product.minutes} min, ${regions.length} regions)`
    )
  }

  console.log(
    `\n${APPLY ? 'Applied' : 'Planned'} ${writes} write${writes === 1 ? '' : 's'}.` +
      (APPLY ? '' : ' Re-run with --apply to execute.')
  )
  console.log(
    '\nBase plans are created in DRAFT. Activate them in Play Console (Monetize → Subscriptions)\n' +
      'or via monetization.subscriptions.basePlans.activate before they can be sold.'
  )
}

// ── entry ───────────────────────────────────────────────────────────────────────

const main = async () => {
  const command = process.argv[2]
  await currentToken() // fail fast on bad credentials
  switch (command) {
    case 'inspect':
      return inspect()
    case 'sync':
      if (!APPLY) console.log('DRY RUN — nothing will be written.\n')
      return sync()
    default:
      console.error('Usage: playstore-products.ts <inspect|sync> [--apply]')
      process.exit(1)
  }
}

main().catch(error => {
  console.error(`\n${error.message}`)
  process.exit(1)
})
