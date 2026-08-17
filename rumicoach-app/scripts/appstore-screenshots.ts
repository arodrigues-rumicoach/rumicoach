#!/usr/bin/env bun
/**
 * Upload screenshots to App Store Connect.
 *
 * Two kinds, and they are not interchangeable:
 *
 *   review — one per in-app purchase, shown to App Review so they can find the purchase in
 *            the app. Pass two images to match each half to the screen it is sold on: the
 *            membership paywall for the subscriptions, the top-up paywall for the
 *            consumables. One image is used for all five.
 *   app    — the screenshots customers see on the store listing.
 *
 * DRY RUN BY DEFAULT. Nothing is written without --apply. A product holds only one review
 * screenshot, so replacing an existing one needs --replace as well.
 *
 *   bun scripts/appstore-screenshots.ts review membership.png topups.png
 *   bun scripts/appstore-screenshots.ts review membership.png topups.png --replace --apply
 *   bun scripts/appstore-screenshots.ts app shot1.png shot2.png --apply
 *   bun scripts/appstore-screenshots.ts app shot1.png --display APP_IPHONE_65 --apply
 *
 * Credentials come from the environment, the same three as
 * scripts/appstore-subscriptions.ts:
 *
 *   ASC_KEY_ID=ABCD123456
 *   ASC_ISSUER_ID=00000000-0000-0000-0000-000000000000
 *   ASC_KEY_PATH=~/.appstoreconnect/AuthKey_ABCD123456.p8
 *
 * The JWT helpers are duplicated from appstore-subscriptions.ts rather than shared:
 * both scripts are standalone by design, runnable straight from a checkout with no
 * build step and no imports beyond the catalog.
 */

import { createHash, createSign } from 'node:crypto'
import { readFileSync, statSync } from 'node:fs'
import { basename } from 'node:path'
import { homedir } from 'node:os'

import { SUBSCRIPTION_PRODUCTS, TOP_UP_PRODUCTS } from '../src/subscriptions/catalog'

const API = 'https://api.appstoreconnect.apple.com'
const APP_ID = '6799373415'

/**
 * Apple accepts one image size to cover the modern iPhone lineup. APP_IPHONE_67 is the
 * 6.7"/6.9" slot — a 1290x2796 or 1320x2868 portrait PNG, which is what an iPhone 16 Pro Max
 * simulator produces. Override with --display for the older slots.
 */
const DEFAULT_DISPLAY_TYPE = 'APP_IPHONE_67'

const APPLY = process.argv.includes('--apply')
/**
 * A product holds at most one review screenshot, so uploading over an existing one 409s with
 * "Screenshot already exists". --replace deletes the current asset first, which is what you
 * want when swapping a placeholder for the real thing.
 */
const REPLACE = process.argv.includes('--replace')
const args = process.argv.slice(2).filter(a => a !== '--apply' && a !== '--replace')
const mode = args.shift()
const displayIndex = args.indexOf('--display')
const displayType = displayIndex >= 0 ? args.splice(displayIndex, 2)[1] : DEFAULT_DISPLAY_TYPE
const files = args

// ── auth ────────────────────────────────────────────────────────────────────────

/**
 * Read statically rather than via process.env[name]: expo/no-dynamic-env-var forbids the
 * dynamic form, because the Babel plugin can only inline EXPO_PUBLIC_* it can see literally.
 */
const need = (name: string, value: string | undefined): string => {
  if (!value) {
    console.error(`Missing ${name}. See the header of this file for the three required vars.`)
    process.exit(1)
  }
  return value
}

const base64url = (input: Buffer | string) =>
  Buffer.from(input).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

/** Node signs ECDSA as DER; JOSE wants the raw r||s pair. */
const derToJose = (der: Buffer): Buffer => {
  let offset = 2
  if (der[1] & 0x80) offset += der[1] & 0x7f
  const readInt = () => {
    if (der[offset++] !== 0x02) throw new Error('Malformed ECDSA signature')
    const len = der[offset++]
    let value = der.subarray(offset, offset + len)
    offset += len
    while (value.length > 32 && value[0] === 0x00) value = value.subarray(1)
    return Buffer.concat([Buffer.alloc(32 - value.length), value])
  }
  return Buffer.concat([readInt(), readInt()])
}

const makeToken = (): string => {
  const now = Math.floor(Date.now() / 1000)
  const header = { alg: 'ES256', kid: need('ASC_KEY_ID', process.env.ASC_KEY_ID), typ: 'JWT' }
  const payload = { iss: need('ASC_ISSUER_ID', process.env.ASC_ISSUER_ID), iat: now, exp: now + 15 * 60, aud: 'appstoreconnect-v1' }
  const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(payload))}`
  const signer = createSign('SHA256')
  signer.update(signingInput)
  const key = readFileSync(need('ASC_KEY_PATH', process.env.ASC_KEY_PATH).replace(/^~/, homedir()), 'utf8')
  return `${signingInput}.${base64url(derToJose(signer.sign(key)))}`
}

// A large upload can outlive one token, so re-mint every 10 minutes.
let token = ''
let tokenMintedAt = 0
const currentToken = (): string => {
  if (!token || Date.now() - tokenMintedAt > 10 * 60 * 1000) {
    token = makeToken()
    tokenMintedAt = Date.now()
  }
  return token
}

const call = async (method: string, path: string, body?: unknown) => {
  const res = await fetch(`${API}${path}`, {
    method,
    headers: { Authorization: `Bearer ${currentToken()}`, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  const json = text ? JSON.parse(text) : null
  if (!res.ok) {
    const detail = (json?.errors ?? []).map((e: any) => e.detail).join('; ') || text.slice(0, 300)
    throw new Error(`${method} ${path} → ${res.status}: ${detail}`)
  }
  return json
}

// ── the three-step asset upload ─────────────────────────────────────────────────

/**
 * Apple's asset flow: reserve the asset to get signed upload URLs, PUT the bytes to each
 * one, then commit with an md5 of the file. Skipping the commit leaves the asset in
 * AWAITING_UPLOAD and it silently never appears — the commit is what publishes it.
 */
const uploadAsset = async (
  reservePath: string,
  reserveBody: unknown,
  file: string,
): Promise<string> => {
  const bytes = readFileSync(file)
  const reserved = await call('POST', reservePath, reserveBody)
  const id: string = reserved.data.id
  const operations = reserved.data.attributes.uploadOperations ?? []

  for (const op of operations) {
    const headers: Record<string, string> = {}
    for (const h of op.requestHeaders ?? []) headers[h.name] = h.value
    const res = await fetch(op.url, {
      method: op.method,
      headers,
      body: bytes.subarray(op.offset, op.offset + op.length),
    })
    if (!res.ok) throw new Error(`Upload chunk failed: ${res.status} ${await res.text()}`)
  }

  const checksum = createHash('md5').update(bytes).digest('hex')
  const type = reserveBody as any
  await call('PATCH', `${reservePath}/${id}`, {
    data: { type: type.data.type, id, attributes: { uploaded: true, sourceFileChecksum: checksum } },
  })
  return id
}

const describe = (file: string) => {
  const { size } = statSync(file)
  return `${basename(file)} (${(size / 1024).toFixed(0)} KB)`
}

// ── review screenshots: one per product ─────────────────────────────────────────

const uploadReviewScreenshots = async (subsFile: string, topUpFile?: string) => {
  // Each product's screenshot should show the screen that product is actually bought on.
  // The membership paywall sells the two subscriptions; the top-up paywall sells the three
  // consumables. Passing one file uses it for all five, which is fine as a placeholder but
  // shows a reviewer the wrong screen for whichever half it does not match.
  const topUps = topUpFile ?? subsFile

  // Subscriptions and consumables use different endpoints and different relationship
  // names, and the consumable one is the v2 resource. Getting either wrong 404s.
  const targets = [
    ...SUBSCRIPTION_PRODUCTS.map(p => ({
      label: p.referenceName,
      appleId: p.appleId,
      file: subsFile,
      path: '/v1/subscriptionAppStoreReviewScreenshots',
      type: 'subscriptionAppStoreReviewScreenshots',
      relationship: 'subscription',
      relatedType: 'subscriptions',
      existing: `/v1/subscriptions/${p.appleId}/appStoreReviewScreenshot`,
    })),
    ...TOP_UP_PRODUCTS.map(p => ({
      label: p.referenceName,
      appleId: p.appleId,
      file: topUps,
      path: '/v1/inAppPurchaseAppStoreReviewScreenshots',
      type: 'inAppPurchaseAppStoreReviewScreenshots',
      relationship: 'inAppPurchaseV2',
      relatedType: 'inAppPurchases',
      // v2 for the read, v1 for the resource itself — Apple splits them.
      existing: `/v2/inAppPurchases/${p.appleId}/appStoreReviewScreenshot`,
    })),
  ]

  console.log(`Subscriptions → ${describe(subsFile)}`)
  console.log(`Consumables   → ${describe(topUps)}\n`)
  for (const t of targets) {
    if (!APPLY) {
      console.log(`  [dry run] ${t.label} (${t.appleId})`)
      continue
    }
    try {
      if (REPLACE) {
        const current = await call('GET', t.existing).catch(() => null)
        const currentId = current?.data?.id
        if (currentId) {
          await call('DELETE', `${t.path}/${currentId}`)
          console.log(`  removed old screenshot for ${t.label}`)
        }
      }
      const id = await uploadAsset(
        t.path,
        {
          data: {
            type: t.type,
            attributes: { fileSize: statSync(t.file).size, fileName: basename(t.file) },
            relationships: { [t.relationship]: { data: { type: t.relatedType, id: t.appleId } } },
          },
        },
        t.file,
      )
      console.log(`  ok  ${t.label} → ${id}`)
    } catch (e: any) {
      console.log(`  FAIL ${t.label}: ${e.message}`)
    }
  }
}

// ── app listing screenshots ────────────────────────────────────────────────────

const uploadAppScreenshots = async (files: string[]) => {
  const versions = await call('GET', `/v1/apps/${APP_ID}/appStoreVersions?limit=1`)
  const version = versions.data?.[0]
  if (!version) throw new Error('No appStoreVersion on the record')

  const locs = await call('GET', `/v1/appStoreVersions/${version.id}/appStoreVersionLocalizations?limit=50`)
  const enUS = (locs.data ?? []).find((l: any) => l.attributes.locale === 'en-US') ?? locs.data?.[0]
  if (!enUS) throw new Error('No appStoreVersionLocalization to attach screenshots to')

  console.log(`Version ${version.attributes.versionString} (${version.attributes.appStoreState}), locale ${enUS.attributes.locale}`)
  console.log(`Display type ${displayType}, ${files.length} file(s)\n`)

  if (!APPLY) {
    for (const f of files) console.log(`  [dry run] ${describe(f)}`)
    return
  }

  // Reuse the set for this display type if it exists; a second set for the same type
  // is rejected.
  const sets = await call('GET', `/v1/appStoreVersionLocalizations/${enUS.id}/appScreenshotSets?limit=20`)
  let set = (sets.data ?? []).find((s: any) => s.attributes.screenshotDisplayType === displayType)
  if (!set) {
    set = (await call('POST', '/v1/appScreenshotSets', {
      data: {
        type: 'appScreenshotSets',
        attributes: { screenshotDisplayType: displayType },
        relationships: { appStoreVersionLocalization: { data: { type: 'appStoreVersionLocalizations', id: enUS.id } } },
      },
    })).data
    console.log(`  created set ${set.id}`)
  }

  for (const file of files) {
    try {
      const id = await uploadAsset(
        '/v1/appScreenshots',
        {
          data: {
            type: 'appScreenshots',
            attributes: { fileSize: statSync(file).size, fileName: basename(file) },
            relationships: { appScreenshotSet: { data: { type: 'appScreenshotSets', id: set.id } } },
          },
        },
        file,
      )
      console.log(`  ok  ${describe(file)} → ${id}`)
    } catch (e: any) {
      console.log(`  FAIL ${describe(file)}: ${e.message}`)
    }
  }
}

// ── entry point ────────────────────────────────────────────────────────────────

if (!mode || !files.length || !['review', 'app'].includes(mode)) {
  console.error('Usage: bun scripts/appstore-screenshots.ts review <membership.png> [topups.png] [--replace] [--apply]')
  console.error('       bun scripts/appstore-screenshots.ts app <file.png...> [--display TYPE] [--apply]')
  process.exit(1)
}

for (const f of files) {
  try {
    statSync(f)
  } catch {
    console.error(`No such file: ${f}`)
    process.exit(1)
  }
}

if (!APPLY) console.log('DRY RUN — pass --apply to upload.\n')

if (mode === 'review') await uploadReviewScreenshots(files[0], files[1])
else await uploadAppScreenshots(files)

if (!APPLY) console.log('\nNothing was uploaded. Re-run with --apply.')
