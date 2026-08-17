// The RevenueCat-hosted paywall, as a plain component.
//
// The paywall itself — layout, copy, colours, and the Terms/Privacy/Restore links App Review
// requires — is configured in the RevenueCat dashboard, not here. Two are published:
// "Rumi Membership" on the `default` offering and "Rumi Minute Top-ups" on `topups`.
// Prices come from the store at runtime, so nothing about pricing is hardcoded in the app.
//
// This file is native-only. Metro picks paywallView.web.tsx on web, which keeps
// react-native-purchases-ui — a native module with no web implementation — out of the web
// bundle entirely.

import RevenueCatUI from 'react-native-purchases-ui'
import type { PaywallViewProps } from './paywallView.types'

export type { PaywallViewProps }

export function PaywallView({
  offering,
  onPurchaseCompleted,
  onRestoreCompleted,
  onDismiss,
}: PaywallViewProps) {
  return (
    <RevenueCatUI.Paywall
      style={{ flex: 1 }}
      options={{ offering }}
      onPurchaseCompleted={({ customerInfo }) => onPurchaseCompleted(customerInfo)}
      onRestoreCompleted={({ customerInfo }) => onRestoreCompleted(customerInfo)}
      onDismiss={onDismiss}
    />
  )
}

/** True when a real paywall can render. Guards the screen against the web stub. */
export const isPaywallSupported = true
