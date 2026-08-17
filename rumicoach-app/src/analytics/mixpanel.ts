import { Platform } from 'react-native'
import { Mixpanel } from 'mixpanel-react-native'
import {
  MPSessionReplay,
  MPSessionReplayConfig,
  MPSessionReplayMask,
  MPDataResidency,
} from '@mixpanel/react-native-session-replay'

/**
 * Mixpanel client, running alongside PostHog while the two are being compared.
 *
 * Only the primitives live here. The event names and properties are in
 * ./index.ts and are sent identically to both tools — a comparison is worthless
 * if each side sees a different shape of the same event.
 */

const token = process.env.EXPO_PUBLIC_MIXPANEL_TOKEN || ''

// EU residency, matching the project. The default is the US ingest host, and
// getting this wrong sends the data to the wrong region rather than failing.
const serverURL = process.env.EXPO_PUBLIC_MIXPANEL_HOST || 'https://api-eu.mixpanel.com'

/**
 * Native only, to match PostHog — so both tools see the same traffic, which is
 * the only way the comparison means anything.
 *
 * The guard below is belt-and-braces: web never reaches this file at all, since
 * ./mixpanel.web.ts shadows it. That stub exists because importing this SDK is
 * enough to break the web build on its own — see the note there.
 */
const isNative = Platform.OS !== 'web'

/**
 * init() is asynchronous, unlike PostHog's constructor, so there is a window at
 * launch where the client exists but isn't usable. Everything below goes through
 * this promise instead of a bare reference, which also preserves ordering:
 * callbacks queued on a settled promise run in the order they were attached, so
 * an identify() issued before init finishes still lands before the events that
 * followed it.
 */
const ready: Promise<Mixpanel | null> = (async () => {
  if (!token || !isNative) return null
  try {
    // true = Mixpanel's own app lifecycle events (installs, opens, updates).
    // On while we're hunting product-market fit; it is the one thing Mixpanel
    // gives for free that would otherwise need hand-written events.
    const mp = new Mixpanel(token, true)
    await mp.init(false, {}, serverURL)
    await startSessionReplay(mp)
    return mp
  } catch {
    // A dead analytics client must never take the app down with it.
    return null
  }
})()

/**
 * Session replay, so the comparison against PostHog covers replay and not just
 * events. Failure here is swallowed separately from the client above: recording
 * is the optional half, and losing it must not cost us the events too.
 */
async function startSessionReplay(mp: Mixpanel): Promise<void> {
  try {
    // Recording is bound to an id at initialize() time, so it has to be the one
    // the SDK settled on — mpIdentify re-points it when the user signs in.
    const distinctId = await mp.getDistinctId()

    await MPSessionReplay.initialize(
      token,
      distinctId,
      new MPSessionReplayConfig({
        serverURL: MPDataResidency.EU,
        // Off, against a default of true. Left on, replays only upload over WiFi
        // and otherwise sit in memory until the app is killed and they are lost —
        // which on a phone is most sessions, and precisely the mobile-only usage
        // we are trying to observe.
        wifiOnly: false,
        recordingSessionsPercent: 100,
        autoStartRecording: true,
        // The SDK masks all four by default. Written out for the same reason as
        // PostHog's: this is a privacy decision about coaching conversations,
        // memories and the user's vision, not a preference, and a future default
        // flipping must not quietly start recording them in clear.
        autoMaskedViews: [
          MPSessionReplayMask.Text,
          MPSessionReplayMask.Image,
          MPSessionReplayMask.Web,
          MPSessionReplayMask.Map,
        ],
      }),
    )
  } catch {
    // Replay is best-effort; events carry on regardless.
  }
}

/** Fire-and-forget. Never rejects, so no call site needs to guard. */
function withClient(fn: (mp: Mixpanel) => void): void {
  ready.then((mp) => { if (mp) fn(mp) }).catch(() => { })
}

export function mpCapture(event: string, properties?: Record<string, unknown>): void {
  withClient((mp) => mp.track(event, properties))
}

export function mpIdentify(userId: string, properties: Record<string, unknown>): void {
  withClient((mp) => {
    mp.identify(userId)
    // Mixpanel splits these: identify() binds the id, people.set() writes the
    // profile. PostHog does both in one call.
    mp.getPeople().set(properties)
    // Recording carries its own copy of the id, bound at initialize() — without
    // this the replays stay filed under the anonymous device id and never join
    // up with the person whose session they show.
    MPSessionReplay.identify(userId).catch(() => { })
  })
}

export function mpRegister(properties: Record<string, unknown>): void {
  withClient((mp) => mp.registerSuperProperties(properties))
}

export function mpReset(): void {
  withClient((mp) => {
    mp.reset()
    // reset() mints a fresh anonymous id; point recording at it too, or the next
    // person to use this device gets their replays filed under the previous
    // user's identity.
    mp.getDistinctId()
      .then((id) => MPSessionReplay.identify(id))
      .catch(() => { })
  })
}

export function mpScreen(name: string, properties?: Record<string, unknown>): void {
  withClient((mp) => mp.autocapture.trackScreenView(name, properties))
}

/** Push anything queued now — used before the identity is torn down. */
export function mpFlush(): void {
  withClient((mp) => mp.flush())
}
