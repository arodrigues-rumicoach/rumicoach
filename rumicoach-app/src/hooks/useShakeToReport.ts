import { useEffect, useRef } from 'react'
import { AppState, Platform } from 'react-native'
import { Accelerometer } from 'expo-sensors'

/** Acceleration magnitude, in g, that counts as a jolt. At rest the reading is
 *  ~1g from gravity alone; walking peaks around 1.3–1.5g. 1.8 sits above normal
 *  handling but well inside what a deliberate shake produces. */
const JOLT_G = 1.8
/** Jolts needed before it counts as a shake. One is a bump — setting the phone
 *  down, a pothole. Three in quick succession is intent. */
const JOLTS_REQUIRED = 3
/** Window the jolts must fall inside. */
const WINDOW_MS = 1000
/** Ignore the sensor for this long after firing, so one shake opens one form
 *  and the tail of the same gesture doesn't reopen it. */
const COOLDOWN_MS = 3000
/** 20Hz. Fast enough to catch a shake, slow enough to be negligible on battery. */
const INTERVAL_MS = 50

/**
 * Calls `onShake` when the user shakes the device.
 *
 * Mobile only: web has no dependable accelerometer (iOS Safari gates
 * DeviceMotion behind a permission prompt tied to a user gesture, which is
 * exactly what we don't have here), so the hook is inert there rather than
 * asking for something it can't reliably get.
 *
 * Listening stops whenever the app leaves the foreground, so a phone shaken in
 * a pocket wakes nothing.
 */
export function useShakeToReport(onShake: () => void, enabled: boolean) {
  // Kept in a ref so changing the handler doesn't tear down the subscription.
  const handler = useRef(onShake)
  useEffect(() => { handler.current = onShake }, [onShake])

  useEffect(() => {
    if (!enabled || Platform.OS === 'web') return

    let subscription: { remove: () => void } | null = null
    let jolts: number[] = []
    let firedAt = 0

    const start = () => {
      if (subscription) return
      Accelerometer.setUpdateInterval(INTERVAL_MS)
      subscription = Accelerometer.addListener(({ x, y, z }) => {
        const magnitude = Math.sqrt(x * x + y * y + z * z)
        if (magnitude < JOLT_G) return

        const now = Date.now()
        if (now - firedAt < COOLDOWN_MS) return

        jolts = [...jolts.filter((t) => now - t < WINDOW_MS), now]
        if (jolts.length >= JOLTS_REQUIRED) {
          jolts = []
          firedAt = now
          handler.current()
        }
      })
    }

    const stop = () => {
      subscription?.remove()
      subscription = null
      jolts = []
    }

    start()
    const appState = AppState.addEventListener('change', (state) => {
      if (state === 'active') start()
      else stop()
    })

    return () => {
      appState.remove()
      stop()
    }
  }, [enabled])
}
