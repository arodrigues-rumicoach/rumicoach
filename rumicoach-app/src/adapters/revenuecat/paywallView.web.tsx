// The RevenueCat paywall, on web.
//
// Native renders react-native-purchases-ui's <PaywallView>; that module has no web build, so
// Metro resolves this file instead. The web SDK draws the *same* dashboard-designed paywall,
// but imperatively — presentPaywall() paints into a DOM node we hand it, and resolves when
// the customer buys something. This file is the adapter between that shape and the React
// component app/paywall.tsx expects, so the screen does not have to know which platform it
// is on.
//
// Web purchases are settled by Paddle. The checkout the paywall opens is Paddle's, and it
// only launches from a domain approved under Paddle → Checkout → Website approval; on an
// unapproved domain the paywall still renders and the checkout is what fails.
//
// A plain <div> rather than a React Native <View>: this file is only ever bundled for web,
// and presentPaywall wants a real HTMLElement.

import { useCallback, useEffect, useRef } from 'react'

import { getWebPurchases } from './web'
import type { PaywallViewProps } from './paywallView.types'
import type { CustomerInfo } from './types'

export type { PaywallViewProps }

/** True: the web SDK renders the same paywall the native one does. */
export const isPaywallSupported = true

/**
 * The paywall is authored once, for a phone, and shared with iOS and Android. Handed the
 * whole browser it takes all of it: measured on a 1345px-wide desktop it laid out at
 * 1345×557, pinned to the top, with ~800px of dead space beneath and the two plan cards
 * stretched to 380px each with a chasm between them.
 *
 * The SDK paints into whatever element it is given, so the fix is the container rather than
 * the design: hand it a phone-shaped column and it lays out the way it was drawn. Everything
 * below is on the host, never on the paywall's own markup — that is generated, and styling
 * into it would break on their next release.
 */
const centeringStyle = {
  position: 'absolute',
  inset: 0,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  // Keeps the card off the edges of a phone-sized window.
  padding: 16,
  // The scrim. Only visible because the column below confines the paywall's own backdrop —
  // without that containment this sits underneath a full-window gradient and does nothing.
  backgroundColor: 'rgba(0, 0, 0, 0.5)',
} as const

/**
 * What makes this read as a modal rather than a page.
 *
 * The paywall paints its own `.viewport-backdrop`: a position: fixed, pointer-events: none
 * gradient that, left alone, covers the entire window from inside this element. That is why the
 * screen looked like a page no matter how narrow the content column was — anything drawn behind
 * it, including a scrim, was painted over.
 *
 * `contain: paint` makes this element the containing block for fixed-position descendants, so
 * that backdrop resolves against the card instead of the viewport. It is the one lever that
 * changes their layout without touching their markup, which is generated and would break on
 * their next release.
 *
 * Caveat worth knowing: the same containment would capture anything else the SDK positions
 * fixed inside this element. Paddle's checkout is documented to build its own full-screen
 * overlay when no purchase target is given, so it should be unaffected — but that path only
 * runs on a real purchase and has not been exercised here.
 *
 * The backdrop is a child rather than a body-level node, so the unmount cleanup still removes
 * it; a fixed layer left behind would tint every screen the customer visited next.
 */
/**
 * Carries the sizing so the close button can be positioned against the card's corner without
 * living inside it — the card scrolls and clips, and a control that scrolls away is no better
 * than no control at all.
 */
const frameStyle = {
  position: 'relative',
  display: 'flex',
  width: '100%',
  // A phone's width. Wider and the plan cards sprawl — measured at full desktop width they
  // stretched to 380px each with a chasm between them; narrower and the longer localisations
  // of the feature bullets wrap to three lines. Below 420px this is simply the viewport, so a
  // phone browser gets the full-bleed layout the design was drawn for, with no stray inset.
  maxWidth: 420,
  maxHeight: '100%',
} as const

const closeButtonStyle = {
  position: 'absolute',
  top: 12,
  right: 12,
  // Above the paywall's own content, which paints inside the sibling card.
  zIndex: 1,
  width: 32,
  height: 32,
  borderRadius: 16,
  border: 'none',
  // Dark enough to stay legible on either theme's header, which is pale in light mode and
  // near-black in dark.
  backgroundColor: 'rgba(0, 0, 0, 0.45)',
  color: '#fff',
  fontSize: 18,
  lineHeight: '18px',
  cursor: 'pointer',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
} as const

const columnStyle = {
  width: '100%',
  maxHeight: '100%',
  // The card scrolls on a short window; the page behind it does not.
  overflow: 'auto',
  contain: 'paint',
  borderRadius: 20,
  boxShadow: '0 24px 60px rgba(0, 0, 0, 0.45)',
} as const

export function PaywallView({
  offering,
  onPurchaseCompleted,
  onDismiss,
  showCloseButton,
}: PaywallViewProps) {
  const hostRef = useRef<HTMLDivElement>(null)
  // The screen navigates away on either callback, so firing both would pop twice. The paywall
  // can reach us down several paths at once — the back button calls onBack while the pending
  // presentPaywall promise rejects with the same cancellation, and the customer may hit Escape
  // on top of that. Only the first wins.
  const settledRef = useRef(false)
  const aliveRef = useRef(true)
  useEffect(() => () => { aliveRef.current = false }, [])

  const settle = useCallback((run: () => void) => {
    if (settledRef.current || !aliveRef.current) return
    settledRef.current = true
    run()
  }, [])

  // Escape closes it. On native this screen is a sheet you can swipe away; on web
  // react-navigation renders the same route as a plain full screen, so without this and the
  // scrim below there is no way out of a paywall whose design carries no close control.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') settle(onDismiss)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [settle, onDismiss])

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    void (async () => {
      const purchases = await getWebPurchases()
      // No API key, or the SDK failed to configure. The screen has already decided a paywall
      // is warranted, so say nothing here and let it close rather than stranding the user on
      // an empty overlay.
      if (!purchases || !aliveRef.current) return settle(onDismiss)

      try {
        const result = await purchases.presentPaywall({
          // undefined, not null: omitting it lets the SDK fall back to the current offering,
          // which is the same contract the native view has.
          offering: offering ?? undefined,
          htmlTarget: host,
          // Annotated because the adapter hands back an untyped instance; the SDK's own
          // signature for this is (closePaywall: () => void) => void.
          onBack: (closePaywall: () => void) => {
            closePaywall()
            settle(onDismiss)
          },
        })
        settle(() => onPurchaseCompleted(result.customerInfo as unknown as CustomerInfo))
      } catch {
        // Cancelling is the ordinary way out of a paywall, and the SDK reports it as a
        // rejection like any other failure. Treat both as a dismissal: the customer is on
        // the screen they were on, and an error toast for "changed my mind" is noise.
        settle(onDismiss)
      }
    })()

    return () => {
      // React owns the div but not what the SDK painted inside it, so clear it by hand.
      host.replaceChildren()
    }
    // Presented once, on mount. Re-presenting on a prop change would tear down a checkout
    // the customer is in the middle of.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    // Clicking the scrim closes it, the way a modal should. Guarded on the target so a click
    // that started inside the card — a mis-drag off a plan button — does not dismiss it.
    <div
      style={centeringStyle}
      onClick={event => {
        if (event.target === event.currentTarget) settle(onDismiss)
      }}
    >
      <div style={frameStyle}>
        <div ref={hostRef} style={columnStyle} />
        {showCloseButton && (
          <button
            type="button"
            aria-label="Close"
            style={closeButtonStyle}
            onClick={() => settle(onDismiss)}
          >
            ✕
          </button>
        )}
      </div>
    </div>
  )
}
