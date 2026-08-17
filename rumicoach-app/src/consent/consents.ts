// Which consents a signed-in user still owes before they can use the app.
//
// Registration collects all three, but registration is not the only door: "Continue with
// Apple" (or Google) on the sign-in screen creates an account and lands straight inside,
// having asked for nothing. Such a user holds no Terms acceptance and no AI consent, and
// neither do accounts created before a given consent existed.
//
// App Review rejected 1.0 under Guidelines 5.1.1(i) and 5.1.2(i) for the AI half of that —
// voice audio reaching Google's AI with no disclosure and no permission, and the rejection
// says plainly that carrying it in the Terms or Privacy Policy is not enough. The missing
// Terms acceptance is a problem of its own, independent of Apple.
//
// Terms and AI are required: the app cannot be used without them. Marketing is optional.

import AsyncStorage from '@react-native-async-storage/async-storage'

import type { User } from '@/api'

/**
 * Each consent is a nullable Go `*time.Time` serialized with `omitempty`, so an ungiven
 * consent is not `null` — the key is absent entirely. Reading a missing key as "not given"
 * is therefore correct rather than a guess.
 *
 * Several spellings are accepted per consent because the JSON tag is chosen in another
 * repo. Today the backend emits the first of each list; the rest cost nothing and remove a
 * whole failure mode, since a rename would otherwise convince the app that nobody has ever
 * consented and show the gate to everyone, forever.
 */
const KEYS = {
  terms: ['termsAndConditionsAcceptedAt', 'TermsAndConditionsAcceptedAt'],
  ai: ['aiAcceptedAt', 'aIAcceptedAt', 'AIAcceptedAt'],
  marketing: ['marketingAcceptedAt', 'MarketingAcceptedAt'],
} as const

export type ConsentKind = keyof typeof KEYS

/** Local mirror, keyed per user. See needsConsent for the one case it covers. */
const localKey = (userId: string) => `consent:${userId}`

const stampedOnServer = (user: User | null, kind: ConsentKind): boolean => {
  if (!user) return false
  const record = user as unknown as Record<string, unknown>
  return KEYS[kind].some(key => {
    const value = record[key]
    return typeof value === 'string' && value.length > 0
  })
}

/** True when the server has both required consents on record. */
export function hasRequiredConsents(user: User | null): boolean {
  return stampedOnServer(user, 'terms') && stampedOnServer(user, 'ai')
}

/**
 * Whether the consent gate must be shown.
 *
 * The server's timestamps are the real answer. The local mirror covers one case: a backend
 * that has not shipped the writable fields yet, where accepting cannot be persisted. Without
 * it the user would agree, be sent back to the gate on the next launch, and have no way
 * through.
 *
 * Absent means "not given", never "assume yes". Asking twice is a small annoyance; running a
 * session on data the user never agreed to send is what got the app rejected.
 */
export async function needsConsent(user: User | null): Promise<boolean> {
  if (!user) return false
  if (hasRequiredConsents(user)) return false
  try {
    return (await AsyncStorage.getItem(localKey(user.id))) === null
  } catch {
    // A storage failure must not become a silent yes.
    return true
  }
}

export async function rememberConsentLocally(userId: string): Promise<void> {
  try {
    await AsyncStorage.setItem(localKey(userId), new Date().toISOString())
  } catch {
    // Best effort. If this fails the user sees the gate again, which is the safe direction.
  }
}
