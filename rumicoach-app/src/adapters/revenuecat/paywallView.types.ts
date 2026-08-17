// Props shared by the native paywall and the web implementation. Kept in its own module so
// neither has to import from the other to describe its own signature.

import type { CustomerInfo, PurchasesOffering } from './types'

export type PaywallViewProps = {
  /** Offering to render. Passing null falls back to RevenueCat's current offering. */
  offering: PurchasesOffering | null
  /** Fired when the purchase succeeds. Refresh entitlements before navigating away. */
  onPurchaseCompleted: (customerInfo: CustomerInfo) => void
  /** Fired when a restore succeeds — a separate path from purchasing. */
  onRestoreCompleted: (customerInfo: CustomerInfo) => void
  /** Fired when the user closes the paywall, including after a cancelled purchase. */
  onDismiss: () => void
  /**
   * Whether to draw our own close control over the paywall.
   *
   * Needed because the two published designs disagree: the top-up paywall carries its own ×,
   * the membership one carries nothing, and a paywall with no way out is a dead end — on web
   * especially, where there is no sheet to swipe away. Drawing one unconditionally would put
   * two × side by side on the top-up paywall, so the caller, which already knows which paywall
   * it asked for, says which one needs ours.
   *
   * The tidier end state is a Close component on the membership design in the RevenueCat
   * dashboard, which would cover iOS and Android too and let this prop go away.
   *
   * Ignored on native, where the screen is a sheet and can always be swiped down.
   */
  showCloseButton?: boolean
}
