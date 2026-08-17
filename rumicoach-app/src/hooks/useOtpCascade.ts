import { useEffect, useRef } from 'react'

const CASCADE_STAGGER_MS = 60
const SINGLE_KEYSTROKE_THRESHOLD = 1

/**
 * Detects when an OTP `value` changes by more than one digit in a single tick
 * (e.g. paste or SMS autofill) and emits per-digit `enteredAt` timestamps with
 * a stagger so cells can cascade their entry animation.
 *
 * Returns:
 *  - `triggerEntry(index)` to be called by each cell to schedule its own
 *    entry animation. Single keystrokes fire instantly; multi-digit changes
 *    are staggered by `CASCADE_STAGGER_MS`.
 */
export function useOtpCascade(value: string) {
  const prevValueRef = useRef(value)
  const enteredAtRef = useRef<Map<number, number>>(new Map())
  const timersRef = useRef<Set<ReturnType<typeof setTimeout>>>(new Set())

  useEffect(() => {
    const prev = prevValueRef.current
    const next = value

    if (next === prev) {
      prevValueRef.current = next
      return
    }

    const addedIndices: number[] = []
    const maxLen = Math.max(prev.length, next.length)
    for (let i = 0; i < maxLen; i++) {
      const prevChar = prev[i] ?? ''
      const nextChar = next[i] ?? ''
      if (nextChar && nextChar !== prevChar) {
        addedIndices.push(i)
      }
    }

    if (addedIndices.length > SINGLE_KEYSTROKE_THRESHOLD) {
      // Clear any in-flight stagger from a previous paste
      timersRef.current.forEach(t => clearTimeout(t))
      timersRef.current.clear()
      enteredAtRef.current.clear()

      const baseTime = Date.now()
      addedIndices.forEach((idx, order) => {
        const at = baseTime + order * CASCADE_STAGGER_MS
        enteredAtRef.current.set(idx, at)
        const timer = setTimeout(() => {
          enteredAtRef.current.set(idx, Date.now())
          timersRef.current.delete(timer)
        }, order * CASCADE_STAGGER_MS)
        timersRef.current.add(timer)
      })
    } else if (addedIndices.length === 1) {
      enteredAtRef.current.set(addedIndices[0], Date.now())
    } else if (next.length < prev.length) {
      // Backspace: clear stale entries beyond new length
      for (let i = next.length; i < prev.length; i++) {
        enteredAtRef.current.delete(i)
      }
    }

    prevValueRef.current = next
  }, [value])

  useEffect(() => {
    return () => {
      timersRef.current.forEach(t => clearTimeout(t))
      timersRef.current.clear()
    }
  }, [])

  const triggerEntry = (index: number): void => {
    enteredAtRef.current.set(index, Date.now())
  }

  const isReadyAt = (index: number, generatedAt: number): boolean => {
    const at = enteredAtRef.current.get(index)
    if (at === undefined) return false
    return at <= generatedAt + 16
  }

  return { triggerEntry, isReadyAt }
}
