// The paywall screen. One route, two paywalls, selected by the `mode` param:
//
//   /paywall                → the membership paywall  (offering: default)
//   /paywall?mode=topup     → the minute top-up paywall (offering: topups)
//
// Both are designed and published in the RevenueCat dashboard, which is also where the
// Terms, Privacy and Restore controls App Review requires live. This screen only picks the
// offering, reports the outcome, and gets out of the way.

import { useEffect, useRef, useState } from 'react'
import { View, ActivityIndicator, StyleSheet } from 'react-native'
import { Text } from 'tamagui'
import { useLocalSearchParams, useNavigation, useRouter } from 'expo-router'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import i18n from '../src/i18n'
import { useRevenueCat } from '../src/hooks/useRevenueCat'
import { useAuth } from '../src/hooks/useAuth'
import { PaywallView, isPaywallSupported } from '../src/adapters/revenuecat/paywallView'
import type { PurchasesOffering } from '../src/adapters/revenuecat'
import { MEMBERSHIP_OFFERING_ID, TOP_UP_OFFERING_ID } from '../src/subscriptions/catalog'
import type { PaywallMode } from '../src/subscriptions/gate'

export default function PaywallScreen() {
  const router = useRouter()
  const navigation = useNavigation()
  const insets = useSafeAreaInsets()
  const { mode } = useLocalSearchParams<{ mode?: string }>()
  const { offerings, isLoading, refreshCustomerInfo, refreshOfferings } = useRevenueCat()
  const { refreshUser } = useAuth()
  // Ref guards the one retry; state drives the UI. Flipping state inside the effect body
  // rather than in the callback would cascade renders.
  const retriedRef = useRef(false)
  const [retryDone, setRetryDone] = useState(false)

  const paywallMode: PaywallMode = mode === 'topup' ? 'topup' : 'membership'
  const offeringId = paywallMode === 'topup' ? TOP_UP_OFFERING_ID : MEMBERSHIP_OFFERING_ID

  // Look the offering up by identifier rather than using `current`. Only one offering can be
  // current, and top-ups are not it — reading `current` here would show the membership
  // paywall to a member who just ran out of minutes.
  const offering: PurchasesOffering | null = offerings?.all?.[offeringId] ?? null

  // Offerings are fetched once at startup. If that call failed or had not finished, retry
  // rather than leaving the user on a dead screen.
  useEffect(() => {
    if (offerings || isLoading || retriedRef.current) return
    retriedRef.current = true
    refreshOfferings().finally(() => setRetryDone(true))
  }, [offerings, isLoading, refreshOfferings])

  // Only claim there is nothing to sell once the retry has actually finished, so the empty
  // state cannot flash while offerings are still in flight.
  const stillLoading = isLoading || (!offerings && !retryDone)

  // back() alone logs "The action 'GO_BACK' was not handled" and does nothing when the paywall
  // is the entry point — a deep link, a shared URL, or a browser reload on /paywall, all of
  // which leave the stack with nothing beneath. The customer is then stuck on a screen no
  // control can leave, so fall through to the journey tab.
  //
  // Asks the navigator rather than router.canGoBack(), which answers for the whole router and
  // returned true here on a direct load — expo-router seats an anchor route under the deep link
  // that back() then refuses to pop. The navigator knows what its own stack can actually do.
  const close = () => {
    if (navigation.canGoBack()) navigation.goBack()
    else router.replace('/(tabs)/journey')
  }

  const onCompleted = async () => {
    // Entitlements and the minute balance both change on purchase, so refresh before the
    // caller re-reads them. The two arrive on different clocks: the entitlement is on the
    // device as soon as the store confirms, while the minutes are credited by RevenueCat's
    // webhook to our backend, so /me can still read as empty for a moment. Nothing blocks
    // on that — the next session start refetches the profile anyway.
    await Promise.allSettled([refreshCustomerInfo(), refreshUser()])
    close()
  }

  if (!isPaywallSupported) {
    return (
      <Centered insetTop={insets.top}>
        <Text fontSize={14} color="$onGlassSecondary" textAlign="center">
          {i18n.t('purchases_unavailable_web') || 'Purchases are not available on web.'}
        </Text>
      </Centered>
    )
  }

  if (!offering) {
    // Still loading, or the offering is genuinely missing — a misconfigured identifier, or
    // no store connection. Either way there is nothing to purchase.
    return (
      <Centered insetTop={insets.top}>
        {stillLoading ? (
          <>
            <ActivityIndicator size="large" color="#10b981" />
            <Text fontSize={14} color="$onGlassSecondary">
              {i18n.t('loading_offerings') || 'Loading available plans...'}
            </Text>
          </>
        ) : (
          <Text fontSize={14} color="$onGlassSecondary" textAlign="center">
            {i18n.t('no_offerings') || 'No plans available at the moment.'}
          </Text>
        )}
      </Centered>
    )
  }

  return (
    <View style={styles.fill}>
      <PaywallView
        offering={offering}
        onPurchaseCompleted={onCompleted}
        onRestoreCompleted={onCompleted}
        onDismiss={close}
        // The top-up design carries its own ×; the membership one carries nothing, and a
        // paywall you cannot leave is a dead end. Asking for ours only where the design is
        // missing one keeps the top-up paywall from showing two.
        showCloseButton={paywallMode === 'membership'}
      />
    </View>
  )
}

function Centered({ children, insetTop }: { children: React.ReactNode; insetTop: number }) {
  return (
    <View style={[styles.centered, { paddingTop: insetTop + 24 }]}>{children}</View>
  )
}

const styles = StyleSheet.create({
  fill: {
    flex: 1,
  },
  centered: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 32,
  },
})
