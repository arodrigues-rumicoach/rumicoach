#!/usr/bin/env bun
/**
 * Push the subscription catalog to Paddle — the billing engine behind the *web* paywall.
 *
 * Third mirror of scripts/appstore-subscriptions.ts, alongside scripts/playstore-products.ts.
 * Same source of truth (src/subscriptions/), same dry-run-by-default contract.
 *
 *   bun scripts/paddle-products.ts inspect          # what exists now
 *   bun scripts/paddle-products.ts sync             # print the plan, change nothing
 *   bun scripts/paddle-products.ts sync --apply     # execute the plan
 *
 * Credentials — one of, and never committed:
 *
 *   PADDLE_API_KEY_PATH=~/.paddle/rumicoach-catalog-sync   # a file holding the key (preferred)
 *   PADDLE_API_KEY=pdl_live_apikey_…                        # the key itself
 *
 * Prefer the file: an exported variable only exists in the shell that exported it, and a live
 * payment credential pasted into ~/.zshrc outlives its usefulness by years. `chmod 600` it and
 * delete it when the catalog is settled.
 *
 * The key needs exactly four permissions: product:read, product:write, price:read, price:write.
 *
 *   PADDLE_ENV=sandbox    # target sandbox-api.paddle.com instead of the live account
 *
 * ── Why a script and not the dashboard ────────────────────────────────────────────
 * Paddle's UI sets non-USD prices one *country* at a time — roughly 200 rows per price. Holding
 * £34.99 and €39.99 across five prices is several hundred hand-typed amounts into a live payment
 * processor. The API takes the same thing as one `unit_price_overrides` array per price, so the
 * whole catalog is five requests and a diff you can read before it runs.
 *
 * ── Paddle's shape vs Apple's and Play's ──────────────────────────────────────────
 * Apple wants a product per plan; Play wants one subscription carrying two base plans; Paddle
 * wants one *product* carrying two *prices*. So `Rumi Membership` holds a monthly and an annual
 * price, and each top-up is its own product with a single one-time price (no billing_cycle).
 *
 * ── Idempotence ──────────────────────────────────────────────────────────────────
 * Everything is matched on `custom_data.rumi_id`, so a re-run after a partial failure resumes
 * rather than duplicating. Products created by hand in the dashboard carry no custom_data, so
 * they are matched on their exact name once and stamped with the id on first sync — that is how
 * the monthly price created before this script existed gets adopted instead of duplicated.
 */

import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'

import {
  SUBSCRIPTION_PRODUCTS,
  TOP_UP_PRODUCTS,
  type Currency,
  type PriceByCurrency,
} from '../src/subscriptions/catalog'
import { PLAN_LOCALIZATIONS, TOP_UP_LOCALIZATIONS } from '../src/subscriptions/appStoreLocalizations'

/**
 * Countries pinned to a currency. Everywhere else pays the USD base, converted by Paddle at
 * checkout — the same bargain we accepted on the App Store and on Play.
 *
 * The euro list is the 20 eurozone members rather than the seven Play pins: on Play each entry
 * is a row we have to maintain, here it is one array element, so there is no reason to be stingy.
 */
const PINNED: Record<Exclude<Currency, 'USD'>, string[]> = {
  GBP: ['GB'],
  EUR: [
    'AT', 'BE', 'HR', 'CY', 'EE', 'FI', 'FR', 'DE', 'GR', 'IE',
    'IT', 'LV', 'LT', 'LU', 'MT', 'NL', 'PT', 'SK', 'SI', 'ES',
  ],
}

/** Paddle's tax category for coaching delivered as software. */
const TAX_CATEGORY = 'standard'

const API = process.env.PADDLE_ENV === 'sandbox'
  ? 'https://sandbox-api.paddle.com'
  : 'https://api.paddle.com'

// ── auth ────────────────────────────────────────────────────────────────────────

const apiKey = (): string => {
  const path = process.env.PADDLE_API_KEY_PATH
  if (path) {
    const key = readFileSync(path.replace(/^~/, homedir()), 'utf8').trim()
    if (!key) throw new Error(`${path} is empty`)
    return key
  }
  const inline = process.env.PADDLE_API_KEY
  if (inline) return inline.trim()
  console.error('No credentials. Set PADDLE_API_KEY_PATH or PADDLE_API_KEY — see the header.')
  return process.exit(1)
}

// ── money ───────────────────────────────────────────────────────────────────────

/**
 * Paddle takes amounts as a string of minor units: 39.99 → "3999". Round before scaling,
 * or floating-point drift turns 30.99 into "3098".
 */
const minorUnits = (amount: number): string => String(Math.round(amount * 100))

/** A USD base with the GBP and EUR pins hung off it, in Paddle's price shape. */
const pricing = (price: PriceByCurrency) => ({
  unit_price: { amount: minorUnits(price.USD), currency_code: 'USD' },
  unit_price_overrides: (Object.keys(PINNED) as (keyof typeof PINNED)[]).map(currency => ({
    country_codes: PINNED[currency],
    unit_price: { amount: minorUnits(price[currency]), currency_code: currency },
  })),
})

// ── transport ───────────────────────────────────────────────────────────────────

const APPLY = process.argv.includes('--apply')
let writes = 0

type Json = Record<string, any>

const request = async (method: string, path: string, body?: Json): Promise<Json> => {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${apiKey()}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
  const text = await res.text()
  let json: Json = {}
  try {
    json = text ? JSON.parse(text) : {}
  } catch {
    throw new Error(`${method} ${path} → ${res.status} (non-JSON)\n  ${text.slice(0, 300)}`)
  }
  if (!res.ok) {
    // Paddle reports field-level problems in error.errors[], and the top-level detail alone
    // ("invalid request") says nothing useful without them.
    const e = json.error ?? {}
    const fields = (e.errors ?? []).map((f: Json) => `${f.field}: ${f.message}`).join('; ')
    throw new Error(`${method} ${path} → ${res.status}\n  ${e.detail ?? text}${fields ? `\n  ${fields}` : ''}`)
  }
  return json
}

/** Paddle paginates at 50 by default and hands back a `next` URL; 200 is its ceiling. */
const list = async (path: string): Promise<Json[]> => {
  const out: Json[] = []
  let next: string | null = `${path}${path.includes('?') ? '&' : '?'}per_page=200`
  while (next) {
    const page: Json = await request('GET', next)
    out.push(...(page.data ?? []))
    const url: string | undefined = page.meta?.pagination?.next
    next = page.meta?.pagination?.has_more && url ? url.replace(API, '') : null
  }
  return out
}

const mutate = async (method: string, path: string, body: Json, label: string): Promise<Json> => {
  writes++
  if (!APPLY) {
    console.log(`  [dry-run] ${label}`)
    return {}
  }
  const result = await request(method, path, body)
  console.log(`  ✓ ${label} → ${result.data?.id ?? ''}`)
  return result.data ?? {}
}

// ── catalog → Paddle ────────────────────────────────────────────────────────────

const MEMBERSHIP_ID = 'membership'

/** The product name Paddle shows at checkout, and the fallback key for adopting hand-made rows. */
const MEMBERSHIP_NAME = 'Rumi Membership'

type PlannedPrice = {
  rumiId: string
  /** Shown at checkout next to the amount. */
  name: string
  /** Internal only — Paddle requires it and never shows it. */
  description: string
  price: PriceByCurrency
  /** Absent for the one-time top-ups. */
  billingCycle?: { interval: 'month' | 'year'; frequency: number }
}

type PlannedProduct = {
  rumiId: string
  name: string
  description: string
  custom: Json
  prices: PlannedPrice[]
}

const plan = (): PlannedProduct[] => {
  const monthly = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'monthly')
  const annual = SUBSCRIPTION_PRODUCTS.find(p => p.plan === 'annual')
  if (!monthly || !annual) throw new Error('Subscription catalog is incomplete')

  const membership: PlannedProduct = {
    rumiId: MEMBERSHIP_ID,
    name: MEMBERSHIP_NAME,
    description: PLAN_LOCALIZATIONS.monthly['en-US'].description,
    custom: { rumi_id: MEMBERSHIP_ID },
    prices: [
      {
        rumiId: 'membership_monthly',
        name: 'Monthly',
        description: monthly.referenceName,
        price: monthly.price,
        billingCycle: { interval: 'month', frequency: 1 },
      },
      {
        rumiId: 'membership_annual',
        name: 'Annual',
        description: annual.referenceName,
        price: annual.price,
        billingCycle: { interval: 'year', frequency: 1 },
      },
    ],
  }

  // Minutes ride along in custom_data so the backend can credit a balance from the webhook
  // without a second table mapping Paddle price ids to minutes.
  const topUps: PlannedProduct[] = TOP_UP_PRODUCTS.map(product => {
    const copy = TOP_UP_LOCALIZATIONS[product.key]['en-US']
    return {
      rumiId: product.key,
      name: copy.displayName,
      description: copy.description,
      custom: { rumi_id: product.key, minutes: String(product.minutes) },
      prices: [
        {
          rumiId: `${product.key}_once`,
          name: `${product.minutes} minutes`,
          description: product.referenceName,
          price: product.price,
        },
      ],
    }
  })

  return [membership, ...topUps]
}

// ── matching ────────────────────────────────────────────────────────────────────

/**
 * Find the Paddle row for a planned one. `rumi_id` first; failing that, the exact name — which
 * only ever matters once, for rows typed into the dashboard before this script existed.
 */
const matchProduct = (existing: Json[], planned: PlannedProduct): Json | undefined =>
  existing.find(p => p.custom_data?.rumi_id === planned.rumiId) ??
  existing.find(p => p.name === planned.name && p.status !== 'archived')

/**
 * Same idea for prices, but the name fallback is the billing cycle: a hand-made price carries
 * whatever description someone typed, while there can only be one monthly and one annual.
 */
const matchPrice = (existing: Json[], planned: PlannedPrice): Json | undefined =>
  existing.find(p => p.custom_data?.rumi_id === planned.rumiId) ??
  existing.find(
    p =>
      p.status !== 'archived' &&
      (p.billing_cycle?.interval ?? null) === (planned.billingCycle?.interval ?? null)
  )

// ── commands ────────────────────────────────────────────────────────────────────

const money = (p: Json): string => {
  const base = `${p.unit_price?.currency_code} ${(Number(p.unit_price?.amount) / 100).toFixed(2)}`
  const overrides = (p.unit_price_overrides ?? [])
    .map((o: Json) => `${o.unit_price.currency_code} ${(Number(o.unit_price.amount) / 100).toFixed(2)}×${o.country_codes.length}`)
    .join(', ')
  return overrides ? `${base} (+ ${overrides})` : `${base} — USD only`
}

const inspect = async () => {
  const products = await list('/products?include=prices&status=active')
  if (!products.length) return console.log('No active products.')
  for (const product of products) {
    console.log(`\n${product.name}  ${product.id}  [${product.status}]`)
    console.log(`  rumi_id: ${product.custom_data?.rumi_id ?? '—'}`)
    for (const price of product.prices ?? []) {
      const cycle = price.billing_cycle
        ? `every ${price.billing_cycle.frequency} ${price.billing_cycle.interval}`
        : 'one-time'
      console.log(`  · ${price.name ?? price.description}  ${price.id}  ${cycle}  ${money(price)}`)
      console.log(`      ${price.status}   rumi_id: ${price.custom_data?.rumi_id ?? '—'}`)
    }
  }
}

const sync = async () => {
  const existing = await list('/products?include=prices')

  for (const planned of plan()) {
    const found = matchProduct(existing, planned)
    const body = {
      name: planned.name,
      description: planned.description,
      tax_category: TAX_CATEGORY,
      custom_data: planned.custom,
    }

    console.log(`\n${planned.name}`)
    let productId: string = found?.id
    if (!found) {
      const created = await mutate('POST', '/products', { ...body, type: 'standard' }, `create product ${planned.name}`)
      // In a dry run there is no id to hang prices off. Keep planning them anyway — the point of
      // the dry run is to show the whole diff, not to stop at the first thing that doesn't exist.
      productId = created.id ?? '<new product>'
    } else {
      console.log(`  = product ${found.id}`)
      const stale =
        found.description !== planned.description ||
        found.custom_data?.rumi_id !== planned.rumiId ||
        found.tax_category !== TAX_CATEGORY
      if (stale) await mutate('PATCH', `/products/${found.id}`, body, `update product ${planned.name}`)
    }

    for (const price of planned.prices) {
      const priceBody: Json = {
        product_id: productId,
        name: price.name,
        description: price.description,
        tax_mode: 'account_setting',
        quantity: { minimum: 1, maximum: 1 },
        custom_data: { rumi_id: price.rumiId },
        ...pricing(price.price),
        ...(price.billingCycle ? { billing_cycle: price.billingCycle } : {}),
      }

      const foundPrice = found ? matchPrice(found.prices ?? [], price) : undefined
      if (!foundPrice) {
        await mutate('POST', '/prices', priceBody, `create price ${price.name} (${money(priceBody)})`)
        continue
      }

      // product_id is fixed at creation; sending it back on a PATCH is rejected.
      const { product_id: _ignored, ...patch } = priceBody
      const same =
        foundPrice.unit_price?.amount === priceBody.unit_price.amount &&
        JSON.stringify(foundPrice.unit_price_overrides ?? []) ===
          JSON.stringify(priceBody.unit_price_overrides) &&
        foundPrice.custom_data?.rumi_id === price.rumiId &&
        foundPrice.name === price.name
      if (same) {
        console.log(`  = price ${price.name} ${foundPrice.id} (${money(foundPrice)})`)
        continue
      }
      console.log(`    was: ${money(foundPrice)}`)
      await mutate('PATCH', `/prices/${foundPrice.id}`, patch, `update price ${price.name} (${money(priceBody)})`)
    }
  }

  console.log(
    `\n${APPLY ? 'Applied' : 'Planned'} ${writes} write${writes === 1 ? '' : 's'}.` +
      (APPLY ? '' : ' Re-run with --apply to execute.')
  )
  if (APPLY) {
    console.log(
      '\nNext: RevenueCat → Product catalog → Products → Import, then attach the imported\n' +
        "products to the 'default' and 'topups' offerings."
    )
  }
}

// ── entry ───────────────────────────────────────────────────────────────────────

const main = async () => {
  const command = process.argv[2]
  apiKey() // fail fast on missing credentials
  switch (command) {
    case 'inspect':
      return inspect()
    case 'sync':
      if (!APPLY) console.log('DRY RUN — nothing will be written.\n')
      return sync()
    default:
      console.error('Usage: paddle-products.ts <inspect|sync> [--apply]')
      process.exit(1)
  }
}

main().catch(error => {
  console.error(`\n${error.message}`)
  process.exit(1)
})
