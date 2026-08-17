// Which paywall to show, and how many minutes are left to show on it.
//
// Two different moments, two different paywalls:
//
//   not a member yet          → the membership paywall  (offering: default)
//   a member, but out of time → the top-up paywall      (offering: topups)
//
// What is deliberately NOT here any more is whether a session may start. This file used
// to carry a canStartSession() mirroring the backend's billing rule, so the app could
// pre-empt a WebSocket handshake refused with an HTTP 402 that no WebSocket client can
// read. Two copies of a billing rule is what that cost, and they drifted: the app put a
// paywall in front of users the server would have let through, mid-onboarding.
//
// The server now refuses over the socket itself, in a form the client can read (see
// SessionContext's WS_CLOSE_INSUFFICIENT_BALANCE), so the rule lives on the server and
// only there. Nothing in the app decides whether a session may start — it asks, and
// handles being told no. Do not reintroduce a local copy of that rule.

import type { CustomerInfo } from '@/adapters/revenuecat'
import type { User } from '@/api'
import { SUBSCRIPTION_PRODUCTS, billingPeriodFromProductId, type BillingPeriod } from './catalog'

export type PaywallMode = 'membership' | 'topup'

export interface MembershipDetails {
  /** Which catalog plan the active entitlement bills on; null when the product
   *  id matches nothing we sell (an old id, a web-billing slug we don't know). */
  plan: BillingPeriod | null
  /** When the membership renews — or, for a cancelled-but-still-active one,
   *  lapses. null when the store did not say (a lifetime grant has none). */
  renewal: { date: Date; willRenew: boolean } | null
  /** The store's own subscription-management page for this customer, straight
   *  from RevenueCat — App Store, Play or the web billing portal, whichever
   *  actually owns the subscription. */
  managementURL: string | null
}

/**
 * Everything the wallet card shows about an active membership, read from the
 * store receipt rather than asserted by us: which plan, when it renews, and
 * where to manage it.
 *
 * Display only. The native SDK sends ISO strings where the web SDK sends
 * Dates, hence the tolerant parsing; across multiple entitlements (there is
 * only one today) the latest expiration wins, matching latestExpirationDate.
 */
export const membershipDetails = (customerInfo: CustomerInfo | null): MembershipDetails => {
  const active = Object.values(customerInfo?.entitlements?.active ?? {})

  let plan: BillingPeriod | null = null
  let renewal: MembershipDetails['renewal'] = null
  for (const entitlement of active) {
    const e = entitlement as {
      productIdentifier?: string
      expirationDate?: string | Date | null
      willRenew?: boolean
    }

    if (plan === null && e.productIdentifier) {
      const known = SUBSCRIPTION_PRODUCTS.find(
        p => p.productId === e.productIdentifier || p.qaProductId === e.productIdentifier,
      )
      // Catalog match first; for ids the catalog does not know (web billing,
      // test stores), the cadence is still legible from the name — read it
      // rather than degrade to the bare "Member" label.
      plan = known?.plan ?? billingPeriodFromProductId(e.productIdentifier)
    }

    if (e.expirationDate) {
      const date = e.expirationDate instanceof Date ? e.expirationDate : new Date(e.expirationDate)
      if (!isNaN(date.getTime()) && (renewal === null || date > renewal.date)) {
        renewal = { date, willRenew: e.willRenew !== false }
      }
    }
  }

  return {
    plan,
    renewal,
    managementURL: (customerInfo as { managementURL?: string | null } | null)?.managementURL ?? null,
  }
}

/**
 * Whether the customer holds any active entitlement.
 *
 * Deliberately not keyed to a named entitlement: there is one membership with no tiers, so
 * any active entitlement means they are a member. If tiers are ever added, this is the
 * function that has to learn about them.
 */
export const hasActiveMembership = (customerInfo: CustomerInfo | null): boolean =>
  Object.keys(customerInfo?.entitlements?.active ?? {}).length > 0

/**
 * Minutes left, or null when the app does not know — a user it has not loaded yet, or a
 * server too old to send the field.
 *
 * Rounded down, and only for display: 30 seconds left is "0 minutes" but is still a
 * positive balance, so the start decision below reads the seconds instead. A negative
 * balance (the last session overran) also displays as 0 rather than as a debt.
 */
export const minutesRemaining = (user: User | null): number | null => {
  if (typeof user?.balanceSeconds !== 'number') return null
  return Math.max(0, Math.floor(user.balanceSeconds / 60))
}

/**
 * Which paywall a refused session should land on.
 *
 * Whether to show one at all is the server's answer, not this function's — it is called
 * only once the socket has already said no. A member has run out and wants minutes
 * (top-up); everyone else needs the membership first.
 */
export const paywallModeFor = (customerInfo: CustomerInfo | null): PaywallMode =>
  hasActiveMembership(customerInfo) ? 'topup' : 'membership'

/** The route for a paywall mode, so callers do not hand-assemble the query string. */
export const paywallRoute = (mode: PaywallMode): string =>
  mode === 'topup' ? '/paywall?mode=topup' : '/paywall'
