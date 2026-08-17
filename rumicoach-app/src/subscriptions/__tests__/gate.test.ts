import { hasActiveMembership, minutesRemaining, paywallModeFor, paywallRoute } from '../gate'
import type { User } from '@/api'
import type { CustomerInfo } from '@/adapters/revenuecat'

const user = (fields: Partial<User>): User => ({ id: 'u1', ...fields })

const member = { entitlements: { active: { membership: {} } } } as unknown as CustomerInfo
const nonMember = { entitlements: { active: {} } } as unknown as CustomerInfo

describe('hasActiveMembership', () => {
  it('is true for any active entitlement, whatever it is called', () => {
    expect(hasActiveMembership(member)).toBe(true)
    expect(hasActiveMembership({ entitlements: { active: { anything: {} } } } as unknown as CustomerInfo)).toBe(true)
  })

  it('is false with no entitlements and with no customer at all', () => {
    expect(hasActiveMembership(nonMember)).toBe(false)
    expect(hasActiveMembership(null)).toBe(false)
  })
})

describe('minutesRemaining', () => {
  it('converts seconds to whole minutes', () => {
    expect(minutesRemaining(user({ balanceSeconds: 9000 }))).toBe(150)
  })

  it('rounds down, so a part-minute never reads as a full one', () => {
    expect(minutesRemaining(user({ balanceSeconds: 119 }))).toBe(1)
  })

  // A session that overran is debited past zero. Displaying "-3 minutes" would read as
  // a debt the user is expected to settle; they just owe nothing and have nothing.
  it('clamps a negative balance to zero', () => {
    expect(minutesRemaining(user({ balanceSeconds: -180 }))).toBe(0)
  })

  it('is null when the server did not send a balance', () => {
    expect(minutesRemaining(user({}))).toBeNull()
    expect(minutesRemaining(null)).toBeNull()
  })
})

// There is no canStartSession here any more, and there should not be one again. The app
// does not decide whether a session may start: it opens the socket and the server either
// grants it or refuses with a reason the client can read. The rule this used to mirror
// lives in the backend's balance.FreeSessionAvailable, and having a second copy of it in
// the client is what once put a paywall in front of half-onboarded users.
describe('paywallModeFor', () => {
  // Only ever called on a refusal that already happened, so there is no "no paywall"
  // answer to give — the question is which one.
  it('offers the membership to a non-member', () => {
    expect(paywallModeFor(nonMember)).toBe('membership')
    expect(paywallModeFor(null)).toBe('membership')
  })

  // A member who ran out already bought the thing the membership paywall sells; sending
  // them there would offer a subscription they hold.
  it('offers top-ups to a member', () => {
    expect(paywallModeFor(member)).toBe('topup')
  })
})

describe('paywallRoute', () => {
  it('maps each mode to its offering', () => {
    expect(paywallRoute('membership')).toBe('/paywall')
    expect(paywallRoute('topup')).toBe('/paywall?mode=topup')
  })
})
