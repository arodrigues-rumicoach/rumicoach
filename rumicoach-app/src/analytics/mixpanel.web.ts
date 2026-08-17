/**
 * Web stub for ./mixpanel.ts — Metro picks this file for the web bundle.
 *
 * The native module guards its calls with Platform.OS, but that isn't enough:
 * `import { Mixpanel } from 'mixpanel-react-native'` executes the package
 * whether or not anything calls it, and its uuid/get-random-values chain has no
 * working implementation under Expo Router's server render (Metro builds the web
 * bundle twice, once with environment=node). That render died with
 * "Cannot read properties of undefined (reading 'v1')" and took `expo start
 * --web` down with it.
 *
 * A platform stub keeps the package out of the web graph entirely, which is also
 * what we want on the merits: analytics is native-only while PostHog and
 * Mixpanel are being compared, so both see the same traffic.
 */

export function mpCapture(_event: string, _properties?: Record<string, unknown>): void { }

export function mpIdentify(_userId: string, _properties: Record<string, unknown>): void { }

export function mpRegister(_properties: Record<string, unknown>): void { }

export function mpReset(): void { }

export function mpScreen(_name: string, _properties?: Record<string, unknown>): void { }

export function mpFlush(): void { }
