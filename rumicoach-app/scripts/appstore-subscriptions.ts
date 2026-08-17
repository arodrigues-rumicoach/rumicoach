#!/usr/bin/env bun
/**
 * Push the subscription catalog to App Store Connect.
 *
 * The catalog in src/subscriptions/ is the source of truth; this script makes App Store
 * Connect match it. It is idempotent — it reads current state first and only issues the
 * calls needed, so re-running after a partial failure resumes rather than duplicating.
 *
 * DRY RUN BY DEFAULT. Nothing is written without --apply.
 *
 *   bun scripts/appstore-subscriptions.ts inspect       # what exists now
 *   bun scripts/appstore-subscriptions.ts price-points   # which prices Apple actually offers
 *   bun scripts/appstore-subscriptions.ts sync           # print the plan, change nothing
 *   bun scripts/appstore-subscriptions.ts sync --apply   # execute the plan
 *
 * Credentials come from the environment, never from the repo:
 *
 *   ASC_KEY_ID=ABCD123456
 *   ASC_ISSUER_ID=00000000-0000-0000-0000-000000000000
 *   ASC_KEY_PATH=~/.appstoreconnect/AuthKey_ABCD123456.p8
 *
 * Generate the key in App Store Connect under Users and Access → Integrations → Team Keys
 * with the App Manager role. The .p8 downloads exactly once; keep it outside the repo.
 */

import { createSign } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'

import {
  PROD_BUNDLE_ID,
  SUBSCRIPTION_GROUP_REFERENCE_NAME,
  SUBSCRIPTION_PRODUCTS,
  TOP_UP_PRODUCTS,
  type Currency,
  type PriceByCurrency,
} from '../src/subscriptions/catalog'
import {
  APP_STORE_LOCALES,
  GROUP_DISPLAY_NAME,
  PLAN_LOCALIZATIONS,
  TOP_UP_LOCALIZATIONS,
} from '../src/subscriptions/appStoreLocalizations'

const API = 'https://api.appstoreconnect.apple.com'

/** Territories where we pin the price by hand; everywhere else keeps Apple's generated price. */
const PINNED_TERRITORIES = {
  USD: ['USA'],
  GBP: ['GBR'],
  // The true eurozone members among our supported locales. Denmark, Sweden, Norway, Poland
  // and Turkey are deliberately absent — Apple bills those in DKK/SEK/NOK/PLN/TRY, so they
  // cannot match the website's EUR price and keep Apple's equalized price instead.
  EUR: ['PRT', 'ESP', 'DEU', 'FRA', 'ITA', 'NLD', 'FIN'],
} as const

// ── auth ────────────────────────────────────────────────────────────────────────

const requireEnv = (name: string): string => {
  const value = process.env[name]
  if (!value) {
    console.error(`Missing ${name}. See the header of this file for the three required vars.`)
    process.exit(1)
  }
  return value
}

const base64url = (input: Buffer | string) =>
  Buffer.from(input).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

/**
 * ES256 JWT, signed by hand so this script needs no dependencies. Node's signer emits
 * DER-encoded ECDSA, but JOSE wants the raw r||s pair, hence the conversion.
 */
const derToJose = (der: Buffer): Buffer => {
  let offset = 2
  if (der[1] & 0x80) offset += der[1] & 0x7f
  const readInt = () => {
    if (der[offset++] !== 0x02) throw new Error('Malformed ECDSA signature')
    const len = der[offset++]
    let value = der.subarray(offset, offset + len)
    offset += len
    // Strip DER's sign padding, then left-pad to the fixed 32 bytes P-256 requires.
    while (value.length > 32 && value[0] === 0x00) value = value.subarray(1)
    return Buffer.concat([Buffer.alloc(32 - value.length), value])
  }
  return Buffer.concat([readInt(), readInt()])
}

const makeToken = (): string => {
  const keyId = requireEnv('ASC_KEY_ID')
  const issuerId = requireEnv('ASC_ISSUER_ID')
  const keyPath = requireEnv('ASC_KEY_PATH').replace(/^~/, homedir())
  const privateKey = readFileSync(keyPath, 'utf8')

  const now = Math.floor(Date.now() / 1000)
  const header = { alg: 'ES256', kid: keyId, typ: 'JWT' }
  // Apple rejects tokens valid for more than 20 minutes.
  const payload = { iss: issuerId, iat: now, exp: now + 15 * 60, aud: 'appstoreconnect-v1' }

  const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(payload))}`
  const signer = createSign('SHA256')
  signer.update(signingInput)
  const signature = derToJose(signer.sign(privateKey))
  return `${signingInput}.${base64url(signature)}`
}

/**
 * A full sync issues well over a hundred calls and can outlive a single token, so mint a
 * fresh one every 10 minutes rather than 401-ing halfway through the localizations.
 */
let token = ''
let tokenMintedAt = 0
const currentToken = (): string => {
  const now = Date.now()
  if (!token || now - tokenMintedAt > 10 * 60 * 1000) {
    token = makeToken()
    tokenMintedAt = now
  }
  return token
}

// ── transport ───────────────────────────────────────────────────────────────────

const APPLY = process.argv.includes('--apply')
let writes = 0

type Json = Record<string, any>

const request = async (method: string, path: string, body?: Json): Promise<Json> => {
  const res = await fetch(path.startsWith('http') ? path : `${API}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${currentToken()}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
  if (res.status === 204) return {}
  const text = await res.text()
  const json = text ? JSON.parse(text) : {}
  if (!res.ok) {
    const detail = json.errors?.map((e: Json) => `${e.title}: ${e.detail}`).join('; ') ?? text
    throw new Error(`${method} ${path} → ${res.status}\n  ${detail}`)
  }
  return json
}

const get = (path: string) => request('GET', path)

/** Every mutation funnels through here, so dry-run coverage cannot be partial. */
const mutate = async (method: string, path: string, body: Json | undefined, label: string) => {
  writes++
  if (!APPLY) {
    console.log(`  [dry-run] ${label}`)
    return { data: { id: `DRY_RUN_${writes}` } } as Json
  }
  const result = await request(method, path, body)
  console.log(`  ✓ ${label}`)
  return result
}

/**
 * GET every page, since 20 locales exceed Apple's default page size. Limit is 50 rather than
 * the 200 some endpoints allow — the cap varies per resource and an over-limit value is a 400,
 * so pay for an extra round trip instead of guessing per endpoint.
 */
const getAll = async (path: string): Promise<Json[]> => {
  const items: Json[] = []
  let next: string | undefined = `${path}${path.includes('?') ? '&' : '?'}limit=50`
  while (next) {
    const page = await get(next)
    items.push(...(page.data ?? []))
    next = page.links?.next
  }
  return items
}

// ── lookups ─────────────────────────────────────────────────────────────────────

const findApp = async () => {
  const apps = await getAll(`/v1/apps?filter[bundleId]=${PROD_BUNDLE_ID}`)
  const app = apps.find(a => a.attributes?.bundleId === PROD_BUNDLE_ID)
  if (!app) throw new Error(`No app record for bundle ID ${PROD_BUNDLE_ID}`)
  return app
}

const findGroup = async (appId: string) => {
  const groups = await getAll(`/v1/apps/${appId}/subscriptionGroups`)
  // The record has exactly one group; reuse it rather than creating a second, which would
  // let a user hold both a monthly and an annual subscription at once.
  return groups[0]
}

// ── commands ────────────────────────────────────────────────────────────────────

const inspect = async () => {
  const app = await findApp()
  console.log(`App: ${app.attributes.name} (${app.id}) — ${app.attributes.bundleId}\n`)

  const groups = await getAll(`/v1/apps/${app.id}/subscriptionGroups`)
  for (const group of groups) {
    console.log(`Group ${group.id}: "${group.attributes.referenceName}"`)
    const locs = await getAll(`/v1/subscriptionGroups/${group.id}/subscriptionGroupLocalizations`)
    console.log(`  localizations: ${locs.length ? locs.map(l => l.attributes.locale).join(', ') : 'none'}`)
    const subs = await getAll(`/v1/subscriptionGroups/${group.id}/subscriptions`)
    for (const sub of subs) {
      const a = sub.attributes
      const subLocs = await getAll(`/v1/subscriptions/${sub.id}/subscriptionLocalizations`)
      // groupLevel 1 is the highest service level. Annual should be above monthly so that
      // switching up takes effect immediately rather than at renewal.
      console.log(
        `  - ${sub.id} "${a.name}" productId=${a.productId} ` +
          `period=${a.subscriptionPeriod} level=${a.groupLevel} state=${a.state} ` +
          `localizations=${subLocs.length}`
      )
    }
  }

  const iaps = await getAll(`/v1/apps/${app.id}/inAppPurchasesV2`)
  console.log(`\nIn-app purchases: ${iaps.length || 'none'}`)
  for (const iap of iaps) {
    const a = iap.attributes
    // Localization count, like the subscriptions above — without it there is no way to
    // confirm a sync actually landed the top-up locales. Note the /v2 path: see sync().
    const locs = await getAll(`/v2/inAppPurchases/${iap.id}/inAppPurchaseLocalizations`)
    console.log(
      `  - ${iap.id} "${a.name}" productId=${a.productId} type=${a.inAppPurchaseType} ` +
        `state=${a.state} localizations=${locs.length}`
    )
  }
}

/**
 * Report the live price in the territories we pin, next to what the catalog says it should be.
 * Reading state back is the only way a wrong price surfaces: Apple's UI shows what you last
 * chose, not whether it matches intent, and one top-up already drifted to the wrong GBP value
 * when Apple's pricePoints endpoint failed mid-edit.
 */
const TERRITORY_CURRENCY: Record<string, Currency> = { USA: 'USD', GBR: 'GBP', DEU: 'EUR' }

const priceAudit = async () => {
  const app = await findApp()
  let mismatches = 0
  let unreadable = 0

  // "could not read" and "wrong" are different findings, and conflating them sends someone
  // hunting a price that may be perfectly fine. Unreadable rows are marked ?, counted
  // separately, and never reported as mismatches.
  const report = (label: string, live: Record<string, number>, want: PriceByCurrency) => {
    let rowOk = true
    let rowUnknown = false
    const cells = Object.entries(TERRITORY_CURRENCY).map(([territory, currency]) => {
      const actual = live[territory]
      const expected = want[currency]
      if (actual === undefined) {
        rowUnknown = true
        return `${territory} ?`
      }
      if (Math.abs(actual - expected) >= 0.005) {
        rowOk = false
        mismatches++
        return `${territory} ${actual} (want ${expected})`
      }
      return `${territory} ${actual}`
    })
    if (rowUnknown) unreadable++
    console.log(`${!rowOk ? '✗' : rowUnknown ? '?' : '✓'} ${label.padEnd(34)} ${cells.join('  ')}`)
  }

  const group = await findGroup(app.id)
  const subs = group ? await getAll(`/v1/subscriptionGroups/${group.id}/subscriptions`) : []
  for (const product of SUBSCRIPTION_PRODUCTS) {
    const sub = subs.find(s => s.attributes.productId === product.productId)
    if (!sub) continue
    const prices = await getAll(
      `/v1/subscriptions/${sub.id}/prices?include=subscriptionPricePoint,territory`
    )
    const live: Record<string, number> = {}
    for (const p of prices) {
      const t = p.relationships?.territory?.data?.id
      const pp = p.relationships?.subscriptionPricePoint?.data?.id
      if (t && pp && TERRITORY_CURRENCY[t]) {
        const point = await get(`/v1/subscriptionPricePoints/${pp}`)
        live[t] = Number(point.data?.attributes?.customerPrice)
      }
    }
    report(product.productId, live, product.price)
  }

  const iaps = await getAll(`/v1/apps/${app.id}/inAppPurchasesV2`)
  for (const product of TOP_UP_PRODUCTS) {
    const iap = iaps.find(i => i.attributes.productId === product.productId)
    if (!iap) continue
    // An IAP's prices hang off a schedule, not the product: iapPriceSchedule → manualPrices →
    // each with a territory and a price point that carries the actual customerPrice.
    const live: Record<string, number> = {}
    // The price schedule's resource id is the in-app purchase id — there is no relationship
    // to traverse, and asking for one 404s with "relationship does not exist".
    const scheduleId = iap.id
    if (scheduleId) {
      const manual = await getAll(
        `/v1/inAppPurchasePriceSchedules/${scheduleId}/manualPrices?include=inAppPurchasePricePoint,territory`
      ).catch(() => [])
      for (const m of manual) {
        const t = m.relationships?.territory?.data?.id
        const pp = m.relationships?.inAppPurchasePricePoint?.data?.id
        if (t && pp && TERRITORY_CURRENCY[t]) {
          const point = await get(`/v1/inAppPurchasePricePoints/${pp}`).catch(() => null)
          const value = Number(point?.data?.attributes?.customerPrice)
          if (Number.isFinite(value)) live[t] = value
        }
      }
    }
    report(product.productId, live, product.price)
  }

  console.log(
    mismatches
      ? `\n${mismatches} price(s) DO NOT match the catalog.`
      : '\nEvery price that could be read matches the catalog.'
  )
  if (unreadable) {
    console.log(
      `${unreadable} product(s) could not be read — Apple exposes no working price relationship\n` +
        'for in-app purchases from this key. Check those in App Store Connect by hand.'
    )
  }
}

/**
 * List the price points Apple actually offers near our targets, in the currencies we pin.
 * This is the empirical answer to whether the website's `.90` prices are selectable.
 */
const pricePoints = async () => {
  const app = await findApp()
  const group = await findGroup(app.id)
  if (!group) throw new Error('No subscription group to read price points from')
  const subs = await getAll(`/v1/subscriptionGroups/${group.id}/subscriptions`)
  if (!subs.length) throw new Error('Need at least one subscription to read price points from')

  const territories = ['USA', 'GBR', 'DEU']
  for (const territory of territories) {
    const points = await getAll(
      `/v1/subscriptions/${subs[0].id}/pricePoints?filter[territory]=${territory}`
    )
    const prices = points
      .map(p => Number(p.attributes.customerPrice))
      .sort((a, b) => a - b)
    const near = (target: number) =>
      prices.filter(p => Math.abs(p - target) / target < 0.06).join(', ') || '(none nearby)'
    console.log(`${territory}: ${prices.length} price points`)
    for (const target of [39.99, 399.9, 15.99, 30.99, 57.99]) {
      const exact = prices.includes(target) ? 'EXACT' : 'not offered'
      console.log(`  ${target}: ${exact} — nearby: ${near(target)}`)
    }
  }
}

const sync = async () => {
  const app = await findApp()
  console.log(`App: ${app.attributes.name} (${app.id})\n`)

  // 1. Reuse the existing group, renaming it to match the catalog.
  let group = await findGroup(app.id)
  if (!group) {
    const created = await mutate(
      'POST',
      '/v1/subscriptionGroups',
      {
        data: {
          type: 'subscriptionGroups',
          attributes: { referenceName: SUBSCRIPTION_GROUP_REFERENCE_NAME },
          relationships: { app: { data: { type: 'apps', id: app.id } } },
        },
      },
      `create group "${SUBSCRIPTION_GROUP_REFERENCE_NAME}"`
    )
    group = created.data
  } else if (group.attributes.referenceName !== SUBSCRIPTION_GROUP_REFERENCE_NAME) {
    await mutate(
      'PATCH',
      `/v1/subscriptionGroups/${group.id}`,
      {
        data: {
          id: group.id,
          type: 'subscriptionGroups',
          attributes: { referenceName: SUBSCRIPTION_GROUP_REFERENCE_NAME },
        },
      },
      `rename group ${group.id} "${group.attributes.referenceName}" → "${SUBSCRIPTION_GROUP_REFERENCE_NAME}"`
    )
  }
  const groupId = group.id

  // 2. Group localizations — the membership name users see managing their subscription.
  const existingGroupLocs = await getAll(
    `/v1/subscriptionGroups/${groupId}/subscriptionGroupLocalizations`
  )
  console.log('Group localizations:')
  for (const locale of APP_STORE_LOCALES) {
    const name = GROUP_DISPLAY_NAME[locale]
    const existing = existingGroupLocs.find(l => l.attributes.locale === locale)
    if (existing?.attributes.name === name) continue
    if (existing) {
      await mutate(
        'PATCH',
        `/v1/subscriptionGroupLocalizations/${existing.id}`,
        { data: { id: existing.id, type: 'subscriptionGroupLocalizations', attributes: { name } } },
        `update ${locale} → "${name}"`
      )
    } else {
      await mutate(
        'POST',
        '/v1/subscriptionGroupLocalizations',
        {
          data: {
            type: 'subscriptionGroupLocalizations',
            attributes: { locale, name },
            relationships: {
              subscriptionGroup: { data: { type: 'subscriptionGroups', id: groupId } },
            },
          },
        },
        `create ${locale} → "${name}"`
      )
    }
  }

  // 3. Retire products whose IDs are not in the catalog. The pre-existing bare `monthly`
  //    is the one this is for. Deleting does not free the identifier — Apple keeps product
  //    IDs reserved account-wide forever — it only stops it cluttering the group.
  const wantedIds = new Set(SUBSCRIPTION_PRODUCTS.map(p => p.productId))
  const existingSubs = await getAll(`/v1/subscriptionGroups/${groupId}/subscriptions`)
  const stale = existingSubs.filter(s => !wantedIds.has(s.attributes.productId))
  if (stale.length) {
    console.log('\nStale subscriptions:')
    for (const sub of stale) {
      if (sub.attributes.state === 'APPROVED') {
        console.log(
          `  ! skipping ${sub.attributes.productId} — state APPROVED, cannot be deleted. ` +
            `Remove it from sale in the UI instead.`
        )
        continue
      }
      await mutate(
        'DELETE',
        `/v1/subscriptions/${sub.id}`,
        undefined,
        `DELETE ${sub.id} productId=${sub.attributes.productId} (${sub.attributes.state})`
      )
    }
  }

  // 4. The two membership plans. In Apple's model level 1 is the *highest* service level, so
  //    annual lands on 1 and monthly on 2. That makes monthly → annual an upgrade (effective
  //    immediately, prorated) and annual → monthly a downgrade (effective at renewal).
  console.log('\nSubscriptions:')
  const subscriptionIds = new Map<string, string>()
  for (const [index, product] of SUBSCRIPTION_PRODUCTS.entries()) {
    const groupLevel = SUBSCRIPTION_PRODUCTS.length - index
    const existing = existingSubs.find(s => s.attributes.productId === product.productId)
    if (existing) {
      subscriptionIds.set(product.plan, existing.id)
      console.log(`  = ${product.productId} already exists (${existing.id})`)
      // Correct the level if it drifted. Deleting a subscription renumbers the survivors,
      // so a group that once held a different plan can end up with annual *below* monthly —
      // which silently turns an upgrade into a downgrade deferred to renewal. The reorder
      // widget in App Store Connect is a pointer-event drag that cannot be automated, but
      // groupLevel is a plain patchable attribute.
      if (existing.attributes.groupLevel !== groupLevel) {
        await mutate(
          'PATCH',
          `/v1/subscriptions/${existing.id}`,
          {
            data: {
              id: existing.id,
              type: 'subscriptions',
              attributes: { groupLevel },
            },
          },
          `set ${product.productId} groupLevel ${existing.attributes.groupLevel} → ${groupLevel}`
        )
      }
      continue
    }
    const created = await mutate(
      'POST',
      '/v1/subscriptions',
      {
        data: {
          type: 'subscriptions',
          attributes: {
            name: product.referenceName,
            productId: product.productId,
            subscriptionPeriod: product.duration === '1 Month' ? 'ONE_MONTH' : 'ONE_YEAR',
            familySharable: false,
            groupLevel,
          },
          relationships: {
            group: { data: { type: 'subscriptionGroups', id: groupId } },
          },
        },
      },
      `create ${product.productId} (${product.duration}, level ${groupLevel})`
    )
    subscriptionIds.set(product.plan, created.data.id)
  }

  // 5. Plan localizations.
  for (const product of SUBSCRIPTION_PRODUCTS) {
    const id = subscriptionIds.get(product.plan)!
    const existing = id.startsWith('DRY_RUN')
      ? []
      : await getAll(`/v1/subscriptions/${id}/subscriptionLocalizations`)
    console.log(`\nLocalizations for ${product.productId}:`)
    for (const locale of APP_STORE_LOCALES) {
      const { displayName, description } = PLAN_LOCALIZATIONS[product.plan][locale]
      const prior = existing.find(l => l.attributes.locale === locale)
      if (prior?.attributes.name === displayName && prior?.attributes.description === description) {
        continue
      }
      if (prior) {
        await mutate(
          'PATCH',
          `/v1/subscriptionLocalizations/${prior.id}`,
          {
            data: {
              id: prior.id,
              type: 'subscriptionLocalizations',
              attributes: { name: displayName, description },
            },
          },
          `update ${locale} → "${displayName}"`
        )
      } else {
        await mutate(
          'POST',
          '/v1/subscriptionLocalizations',
          {
            data: {
              type: 'subscriptionLocalizations',
              attributes: { locale, name: displayName, description },
              relationships: {
                subscription: { data: { type: 'subscriptions', id } },
              },
            },
          },
          `create ${locale} → "${displayName}"`
        )
      }
    }
  }

  // 6. Top-ups, as consumable in-app purchases.
  console.log('\nTop-ups:')
  const existingIaps = await getAll(`/v1/apps/${app.id}/inAppPurchasesV2`)
  const topUpIds = new Map<string, string>()
  for (const product of TOP_UP_PRODUCTS) {
    const existing = existingIaps.find(i => i.attributes.productId === product.productId)
    if (existing) {
      topUpIds.set(product.key, existing.id)
      console.log(`  = ${product.productId} already exists (${existing.id})`)
      continue
    }
    const created = await mutate(
      'POST',
      '/v2/inAppPurchases',
      {
        data: {
          type: 'inAppPurchases',
          attributes: {
            name: product.referenceName,
            productId: product.productId,
            inAppPurchaseType: 'CONSUMABLE',
            reviewNote: `Adds ${product.minutes} voice coaching minutes to the account balance. Minutes roll over and do not expire.`,
          },
          relationships: { app: { data: { type: 'apps', id: app.id } } },
        },
      },
      `create ${product.productId} (consumable, ${product.minutes} min)`
    )
    topUpIds.set(product.key, created.data.id)
  }

  for (const product of TOP_UP_PRODUCTS) {
    const id = topUpIds.get(product.key)!
    // Reading the relationship is /v2 while the localization resource itself is /v1 — an
    // asymmetry in Apple's API, not a typo. /v1/inAppPurchases/{id}/inAppPurchaseLocalizations
    // returns 404 "The relationship 'inAppPurchaseLocalizations' does not exist".
    const existing = id.startsWith('DRY_RUN')
      ? []
      : await getAll(`/v2/inAppPurchases/${id}/inAppPurchaseLocalizations`)
    console.log(`\nLocalizations for ${product.productId}:`)
    for (const locale of APP_STORE_LOCALES) {
      const { displayName, description } = TOP_UP_LOCALIZATIONS[product.key][locale]
      const prior = existing.find(l => l.attributes.locale === locale)
      if (prior?.attributes.name === displayName && prior?.attributes.description === description) {
        continue
      }
      if (prior) {
        await mutate(
          'PATCH',
          `/v1/inAppPurchaseLocalizations/${prior.id}`,
          {
            data: {
              id: prior.id,
              type: 'inAppPurchaseLocalizations',
              attributes: { name: displayName, description },
            },
          },
          `update ${locale} → "${displayName}"`
        )
      } else {
        await mutate(
          'POST',
          '/v1/inAppPurchaseLocalizations',
          {
            data: {
              type: 'inAppPurchaseLocalizations',
              attributes: { locale, name: displayName, description },
              relationships: {
                inAppPurchaseV2: { data: { type: 'inAppPurchases', id } },
              },
            },
          },
          `create ${locale} → "${displayName}"`
        )
      }
    }
  }

  console.log(
    `\n${APPLY ? 'Applied' : 'Planned'} ${writes} write${writes === 1 ? '' : 's'}.` +
      (APPLY ? '' : ' Re-run with --apply to execute.')
  )
  console.log(
    '\nThis script does not manage pricing — it only creates products and localizations.\n' +
      'Prices were set by hand in App Store Connect with Germany as the base territory, and\n' +
      `pinned in ${PINNED_TERRITORIES.USD.join(',')} (USD), ${PINNED_TERRITORIES.GBP.join(',')} (GBP) and ` +
      `${PINNED_TERRITORIES.EUR.join(',')} (EUR).\n` +
      'See docs/APP_STORE_SUBSCRIPTIONS.md. To inspect what Apple offers:\n' +
      '  bun scripts/appstore-subscriptions.ts price-points'
  )
}

// ── entry ───────────────────────────────────────────────────────────────────────

const main = async () => {
  const command = process.argv[2]
  // Mint up front so missing credentials fail immediately, not mid-run.
  currentToken()
  switch (command) {
    case 'prices':
      return priceAudit()
    case 'inspect':
      return inspect()
    case 'price-points':
      return pricePoints()
    case 'sync':
      if (!APPLY) console.log('DRY RUN — nothing will be written.\n')
      return sync()
    default:
      console.error('Usage: appstore-subscriptions.ts <inspect|prices|price-points|sync> [--apply]')
      process.exit(1)
  }
}

main().catch(error => {
  console.error(`\n${error.message}`)
  process.exit(1)
})
