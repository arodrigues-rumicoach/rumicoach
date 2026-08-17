import { Dimensions, Platform } from 'react-native'
import Constants from 'expo-constants'
import * as Device from 'expo-device'
import { getLocale } from '@/i18n/instance'

/** Diagnostics attached to a feedback report. Every field is optional: web and
 *  mobile know different things, and the server takes whatever arrives. */
export interface FeedbackContext {
  appVersion?: string
  buildNumber?: string
  osVersion?: string
  deviceModel?: string
  locale?: string
  timezone?: string
  screen?: string
  screenSize?: string
  userAgent?: string
}

function timezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined
  } catch {
    return undefined
  }
}

/** On web there is no OS version to read, so the field carries the browser and
 *  its version instead — the spec has no separate field for it. */
function webBrowser(): string | undefined {
  if (Platform.OS !== 'web' || typeof navigator === 'undefined') return undefined
  const ua = navigator.userAgent
  const m =
    ua.match(/(Firefox|Edg|OPR)\/([\d.]+)/) ||
    ua.match(/(Chrome)\/([\d.]+)/) ||
    ua.match(/Version\/([\d.]+).*(Safari)/)
  if (!m) return undefined
  // The Safari branch captures version first, browser second.
  return m[2] === 'Safari' ? `Safari ${m[1]}` : `${m[1] === 'Edg' ? 'Edge' : m[1]} ${m[2]}`
}

/**
 * Collect diagnostics for a feedback report.
 *
 * Call this when the form OPENS, not when it submits: `screen` is meant to say
 * where the user hit the problem, and by the time they finish typing they may
 * have navigated somewhere else.
 *
 * `screen` and `size` are passed in rather than read here: the route wants
 * expo-router's segments (`(tabs)/journey`), which only a hook exposes, and
 * `Dimensions.get()` reports 0x0 on web when called outside a render — which
 * silently produced a useless "0x0" in every web report.
 */
export function collectFeedbackContext(
  screen?: string,
  size?: { width: number; height: number },
): FeedbackContext {
  const { width, height } = size ?? Dimensions.get('window')
  const isWeb = Platform.OS === 'web'

  return {
    appVersion: Constants.expoConfig?.version ?? undefined,
    buildNumber: Platform.select({
      ios: Constants.expoConfig?.ios?.buildNumber ?? undefined,
      android: Constants.expoConfig?.android?.versionCode?.toString() ?? undefined,
      default: undefined,
    }),
    osVersion: isWeb
      ? webBrowser()
      : [Device.osName, Device.osVersion].filter(Boolean).join(' ') || undefined,
    deviceModel: isWeb ? undefined : Device.modelId ?? Device.modelName ?? undefined,
    // The app's language, which is not always the device's — it matters when the
    // report is about a translation.
    locale: getLocale(),
    timezone: timezone(),
    screen,
    // Omitted rather than sent as "0x0": a zero here means we failed to read the
    // viewport, and a wrong size is worse than none when someone is trying to
    // reproduce a layout bug from it.
    screenSize: width > 0 && height > 0 ? `${Math.round(width)}x${Math.round(height)}` : undefined,
    userAgent: isWeb && typeof navigator !== 'undefined' ? navigator.userAgent : undefined,
  }
}
