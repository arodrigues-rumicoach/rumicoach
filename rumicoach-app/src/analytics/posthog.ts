import { Platform } from 'react-native'
import PostHog from 'posthog-react-native'

/**
 * PostHog client and primitives.
 *
 * Only the transport lives here. Event names and properties are in ./index.ts
 * and go to both this and Mixpanel — the two are being compared, and that only
 * works if each sees the same events with the same shape.
 *
 * Kept as a standalone instance rather than letting <PostHogProvider apiKey=…>
 * build one, so that non-React code — AuthContext, SessionContext, the screen
 * tracker — can reach it without being inside the provider tree. The provider
 * still wraps the app and is handed this same instance via its `client` prop.
 */

const apiKey = process.env.EXPO_PUBLIC_POSTHOG_KEY || ''

// EU cloud, because the project lives on eu.posthog.com. Sending to the US host
// would put the data in the wrong region — this app already routes its own API
// by region for the same reason.
const host = process.env.EXPO_PUBLIC_POSTHOG_HOST || 'https://eu.i.posthog.com'

/**
 * Native only, deliberately.
 *
 * This SDK records session replay through native iOS/Android views, and its
 * storage layer expects a filesystem — on web `new PostHog()` throws
 * "No storage available" during render, which takes the whole app down with it.
 * PostHog's answer for browsers is posthog-js, a separate SDK; wiring that up is
 * its own job. Until then the web build reports nothing.
 */
const isNative = Platform.OS !== 'web'

/**
 * Null when there is no key or we're on web. The former is the normal state for
 * local dev and for any build whose pipeline doesn't export one. Every helper
 * below no-ops on null, so a missing key degrades to "no analytics" instead of a
 * crash on launch.
 */
export const posthog = apiKey && isNative
  ? new PostHog(apiKey, {
      host,
      enableSessionReplay: true,
      // Survives an app restart, so a person who backgrounds the app mid-session
      // and returns reads as one session rather than two. Matters here because
      // coaching conversations get interrupted.
      enablePersistSessionIdAcrossRestart: true,
      sessionReplayConfig: {
        // These three are already the SDK's defaults. They are written out anyway
        // because they are a privacy decision, not a preference: this app puts
        // coaching conversations, memories, insights and the user's vision on
        // screen, and replay on mobile is a screenshot of whatever is showing.
        // Spelling them out means a future SDK default flipping to false cannot
        // silently start recording that content in clear.
        maskAllTextInputs: true,
        maskAllImages: true,
        maskAllSandboxedViews: true,
        // On while we're hunting product-market fit: these carry the API errors
        // and request timings behind a stalled screen, which is most of what
        // makes a replay worth watching. Release builds don't log tokens — the
        // token logging in AuthContext is behind __DEV__.
        captureLog: true,
        captureNetworkTelemetry: true,
      },
    })
  : null

export function phCapture(event: string, properties?: Record<string, unknown>): void {
  posthog?.capture(event, properties as never)
}

export function phIdentify(userId: string, properties: Record<string, unknown>): void {
  posthog?.identify(userId, properties as never)
}

export function phRegister(properties: Record<string, unknown>): void {
  posthog?.register(properties as never)
}

export function phReset(): void {
  posthog?.reset()
}

export function phScreen(name: string, properties?: Record<string, unknown>): void {
  posthog?.screen(name, properties as never)
}

export function phFlush(): void {
  posthog?.flush()
}
