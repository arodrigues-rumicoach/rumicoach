// Capture Rumi Coach screens as 1080x1920 PNGs for the Play store listing.
//
// 360x640 CSS at dSF 3 -> 1080x1920, exactly 16:9. Play rejects anything where the long
// side is more than twice the short side, which is why this is not the usual tall-phone
// 9:19.5 viewport.
//
// Runs against the e2e mock server (port 3001) + expo web (8084), so no real account is
// involved: the fixture user's OTP is a constant.

import { chromium } from 'playwright'
import { mkdirSync } from 'node:fs'

const OUT = process.argv[2] || './out'
const BASE = 'http://localhost:8084'
const EMAIL = 'e2e-test@rumi.coach'
const OTP = '123456'

mkdirSync(OUT, { recursive: true })

const browser = await chromium.launch()
const page = await browser.newPage({
  viewport: { width: 360, height: 640 },
  deviceScaleFactor: 3,
  isMobile: true,
  hasTouch: true,
})

const shot = async (name) => {
  await page.waitForTimeout(1200)
  await page.screenshot({ path: `${OUT}/${name}.png` })
  console.log(`  saved ${name}.png`)
}

const step = async (label, fn) => {
  process.stdout.write(`${label}… `)
  try {
    await fn()
    console.log('ok')
  } catch (e) {
    console.log(`FAILED: ${e.message.split('\n')[0]}`)
    await page.screenshot({ path: `${OUT}/_fail-${label.replace(/\W+/g, '-')}.png` })
    throw e
  }
}

await step('load', async () => {
  await page.goto(BASE, { waitUntil: 'networkidle', timeout: 120_000 })
  await page.waitForTimeout(3000)
})

await shot('00-signin')

await step('use-email', async () => {
  await page.getByText('Use Email', { exact: false }).first().click()
  await page.waitForTimeout(1500)
})
await shot('01-email-form')

// Enter does not submit this form — there is an explicit CONTINUE button, and without
// clicking it the next fill() just overwrites the email field with the OTP.
// The two steps use different labels: CONTINUE for the email, "Verify & Sign In" for the
// code. One regex covers both so the flow does not care which screen it is on.
const submit = async () => {
  await page.getByText(/CONTINUE|Verify\s*&\s*Sign\s*In/i).first().click()
}

await step('enter-email', async () => {
  const input = page.locator('input').first()
  await input.click()
  await input.fill(EMAIL)
  await page.waitForTimeout(400)
  await submit()
  await page.waitForTimeout(3000)
})
await shot('02-otp')

await step('enter-otp', async () => {
  const inputs = page.locator('input')
  const n = await inputs.count()
  if (n > 1) {
    for (let i = 0; i < OTP.length && i < n; i++) await inputs.nth(i).fill(OTP[i])
  } else {
    await inputs.first().click()
    await inputs.first().fill(OTP)
  }
  await page.waitForTimeout(500)
  // The form submits itself once the sixth digit lands, so the button is often already
  // gone. Clicking is a fallback for the case where it is not.
  await submit().catch(() => {})
  await page.waitForTimeout(8000)
})
await shot('10-journey')

// Navigate through the tab bar rather than page.goto: a full reload drops the session and
// bounces back to sign-in.
const tabs = [
  ['11-talk', /^talk$/i],
  ['12-memories', /^memories$/i],
  ['13-profile', /^profile$/i],
  ['14-journey2', /^journey$/i],
]

for (const [name, label] of tabs) {
  try {
    process.stdout.write(`tab ${label}… `)
    await page.getByText(label).first().click()
    await page.waitForTimeout(3500)
    await page.screenshot({ path: `${OUT}/${name}.png` })
    console.log('ok')
  } catch (e) {
    console.log(`skip (${e.message.split('\n')[0]})`)
  }
}

// A couple of screens that only exist as routes. If the session does not survive a reload
// these come out as the sign-in screen, which the review step below will catch.
for (const [name, route] of [['15-commitments', '/commitments'], ['16-paywall', '/paywall']]) {
  try {
    process.stdout.write(`route ${route}… `)
    await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 60_000 })
    await page.waitForTimeout(3000)
    await page.screenshot({ path: `${OUT}/${name}.png` })
    console.log('ok')
  } catch (e) {
    console.log(`skip (${e.message.split('\n')[0]})`)
  }
}

await browser.close()
console.log('done')
