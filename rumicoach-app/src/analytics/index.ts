import type { User } from '@/api'
import { phCapture, phIdentify, phRegister, phReset, phScreen, phFlush } from './posthog'
import { mpCapture, mpIdentify, mpRegister, mpReset, mpScreen, mpFlush } from './mixpanel'

/**
 * The app's analytics API.
 *
 * PostHog and Mixpanel both run, on trial against each other for a few months.
 * Every event goes to both with the same name and the same properties — a
 * comparison where each tool sees a different shape answers nothing. That also
 * means the naming follows one convention (snake_case, object_verb, past tense)
 * rather than each vendor's house style, so the same funnel can be built twice
 * and the numbers put side by side.
 *
 * Call sites import from here, never from ./posthog or ./mixpanel directly, so
 * dropping the loser at the end of the trial is a change to this file alone.
 */

/** What both SDKs accept as a property value — notably not `undefined`. */
type PropValue = string | number | boolean | null

/**
 * Drop undefined keys.
 *
 * Most of what we send comes off an optional field on User, and PostHog treats a
 * key present-but-undefined differently from an absent one — the first can
 * overwrite a good value on the person with nothing.
 */
function clean(props: Record<string, PropValue | undefined>): Record<string, PropValue> {
  const out: Record<string, PropValue> = {}
  for (const [k, v] of Object.entries(props)) {
    if (v !== undefined) out[k] = v
  }
  return out
}

function capture(event: string, properties?: Record<string, PropValue | undefined>): void {
  const props = properties ? clean(properties) : undefined
  phCapture(event, props)
  mpCapture(event, props)
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

/**
 * Date of birth is deliberately reduced to a bracket before it leaves the app.
 * The analytical question is "do older users stick around longer", which a
 * bracket answers, and a raw birth date is a direct identifier that a bracket
 * isn't.
 */
function ageBracket(dateOfBirth?: string | null): string | undefined {
  if (!dateOfBirth) return undefined
  const born = new Date(dateOfBirth)
  if (Number.isNaN(born.getTime())) return undefined
  const age = Math.floor((Date.now() - born.getTime()) / 31_557_600_000)
  if (age < 18) return 'under_18'
  if (age < 25) return '18_24'
  if (age < 35) return '25_34'
  if (age < 45) return '35_44'
  if (age < 55) return '45_54'
  if (age < 65) return '55_64'
  return '65_plus'
}

/**
 * Link everything from here on to the signed-in user, and attach the dimensions
 * worth slicing cohorts by. Called on every user change, so a coach switch or a
 * language change updates the profile rather than going stale.
 */
export function identifyUser(user: User): void {
  const profile = clean({
    coach: user.coach,
    coachGender: user.coachGender,
    coachVoice: user.coachVoice,
    gender: user.gender,
    ageBracket: ageBracket(user.dateOfBirth),
    preferredLanguage: user.preferredLanguage,
    country: user.country,
    region: user.region,
    theme: user.theme,
    focusArea: user.focusArea,
    chatRetentionDays: user.chatHistoryRetentionDays,
    // Two flags worth having as cohorts rather than derived from events every
    // time: "finished setup" and "has mapped their life balance".
    //
    // Setup is complete once the three details the onboarding intro collects are in —
    // the same test the server routes on. This read an isInitialSetup field that the
    // API has never actually sent, so it was undefined for everybody and every user
    // was reported as set up, including the ones who never finished.
    setupComplete: !!user.dateOfBirth && !!user.gender && !!user.country,
    hasWheelOfLife: !!user.wheelOfLife && Object.keys(user.wheelOfLife).length > 0,
    signedUpAt: user.createdAt,
  })

  phIdentify(user.id, profile)
  mpIdentify(user.id, profile)

  // Region rides on every subsequent event too, so funnels can be split by it
  // without joining back to the person.
  if (user.region) {
    phRegister({ region: user.region })
    mpRegister({ region: user.region })
  }
}

/**
 * Drop the association on logout, so a second person signing in on the same
 * device does not inherit the first one's timeline.
 */
export function resetUser(): void {
  phReset()
  mpReset()
}

// ---------------------------------------------------------------------------
// Screens
// ---------------------------------------------------------------------------

/**
 * expo-router segments come through as '(tabs)/journey' or '(settings)/manageData'.
 * The group parentheses are a routing detail nobody reading a report cares about,
 * so the name is cleaned up and the raw path kept as a property for debugging.
 */
export function captureScreen(path: string): void {
  const name = path
    .replace(/\((auth|tabs|settings)\)\//g, '')
    .replace(/^\(|\)$/g, '')
  const screen = name || path
  phScreen(screen, { path })
  mpScreen(screen, { path })
}

// ---------------------------------------------------------------------------
// Signup and activation
// ---------------------------------------------------------------------------

export function trackSignupCompleted(
  method: 'google' | 'apple' | 'email' | 'phone',
  dataRegion: string,
  marketingOptIn: boolean,
): void {
  capture('signup_completed', { method, dataRegion, marketingOptIn })
}

/** Profile setup finished — the wizard is done and the app is reachable. */
export function trackOnboardingCompleted(): void {
  capture('onboarding_completed')
}

export function trackLogin(method: 'google' | 'apple' | 'email' | 'phone' | 'restored'): void {
  capture('login', { method })
}

/** Which step of the signup wizard someone reached — this is the drop-off funnel. */
export function trackSignupStep(step: string): void {
  capture('signup_step_viewed', { step })
}

// ---------------------------------------------------------------------------
// The core loop: talking to the coach
// ---------------------------------------------------------------------------

export function trackSessionStarted(sessionType: string, isRetry: boolean): void {
  capture('session_started', { sessionType, isRetry })
}

/**
 * `completed` distinguishes someone who let the conversation finish from someone
 * who hung up: the first is the product working, the second is the funnel leaking,
 * and without the flag both look identical.
 */
export function trackSessionEnded(
  sessionType: string | undefined,
  durationSeconds: number,
  completed: boolean,
): void {
  capture('session_ended', { sessionType, durationSeconds, completed })
}

export function trackSessionRated(rating: number, hasText: boolean): void {
  capture('session_rated', { rating, hasText })
}

/** Tapping a card on Journey — the intent, before the session actually connects. */
export function trackSessionStartTapped(sessionType: string, origin: string): void {
  capture('session_start_tapped', { sessionType, origin })
}

// ---------------------------------------------------------------------------
// Habit
// ---------------------------------------------------------------------------

export function trackCommitmentToggled(completed: boolean, type?: string): void {
  capture('commitment_toggled', { completed, type })
}

export function trackMemoryDeleted(category: string): void {
  capture('memory_deleted', { category })
}

// ---------------------------------------------------------------------------
// Channels
// ---------------------------------------------------------------------------

export function trackIntegrationLinkStarted(provider: string): void {
  capture('integration_link_started', { provider })
}

export function trackIntegrationUnlinked(provider: string): void {
  capture('integration_unlinked', { provider })
}

export function trackReplyModeChanged(provider: string, mode: string): void {
  capture('reply_mode_changed', { provider, mode })
}

// ---------------------------------------------------------------------------
// Trust signals
//
// People who start poking at data retention and deletion are often on their way
// out, and these fire days before the account does.
// ---------------------------------------------------------------------------

export function trackDataExportRequested(delivery: 'download' | 'email'): void {
  capture('data_export_requested', { delivery })
}

export function trackDataDeleted(scope: string): void {
  capture('data_deleted', { scope })
}

export function trackChatRetentionChanged(days: number): void {
  capture('chat_retention_changed', { days })
}

export function trackAccountDeleted(): void {
  capture('account_deleted')
  // Push before reset() drops the identity, or the event lands anonymous. Both
  // SDKs batch, so both need telling.
  phFlush()
  mpFlush()
}

// ---------------------------------------------------------------------------
// Feedback
// ---------------------------------------------------------------------------

export function trackFeedbackSubmitted(
  type: string,
  hasImages: boolean,
  viaShake: boolean,
): void {
  capture('feedback_submitted', { type, hasImages, viaShake })
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

export function trackSettingChanged(setting: string, value: PropValue): void {
  capture('setting_changed', { setting, value })
}
