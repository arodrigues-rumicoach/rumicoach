/**
 * Workaround for Tamagui v2 bug #3968:
 * `console.error(..., props)` in createComponent crashes Expo LogBox when props
 * contain circular references (theme Provider context). The warning is usually a
 * false positive caused by JSX whitespace between Tamagui components.
 *
 * This patch safely serializes arguments passed to console.error and suppresses
 * the specific "Unexpected text node" warning.
 */

const CIRCULAR_REPLACEMENT = '[Circular]'

function safeStringify(value: unknown, space?: string | number): string {
  const seen = new WeakSet()
  return JSON.stringify(
    value,
    (_key, val) => {
      if (typeof val === 'object' && val !== null) {
        if (seen.has(val)) {
          return CIRCULAR_REPLACEMENT
        }
        seen.add(val)
      }
      return val
    },
    space
  )
}

function isUnexpectedTextNodeWarning(args: unknown[]): boolean {
  return (
    typeof args[0] === 'string' &&
    args[0].includes('Unexpected text node:') &&
    args[0].includes('A text node cannot be a child of')
  )
}

export function patchConsoleError(): void {
  if (typeof console === 'undefined' || !console.error) return

  const originalError = console.error

  console.error = (...args: unknown[]) => {
    // Suppress the false-positive whitespace-text-node warning from Tamagui.
    if (isUnexpectedTextNodeWarning(args)) {
      return
    }

    // For all other errors, replace circular references so LogBox/Metro don't
    // throw "Converting circular structure to JSON".
    const safeArgs = args.map((arg) => {
      if (typeof arg === 'object' && arg !== null) {
        try {
          // Validate that the object can be stringified; if not, return a safe
          // clone with circular refs replaced.
          JSON.stringify(arg)
          return arg
        } catch {
          return JSON.parse(safeStringify(arg))
        }
      }
      return arg
    })

    originalError.apply(console, safeArgs)
  }
}

