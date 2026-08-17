import { createContext, useState, useEffect, useCallback, useRef, useMemo, type ReactNode } from 'react'
import { getRevenueCatAdapter, type PurchasesPackage, type CustomerInfo, type PurchasesOfferings } from '../adapters/revenuecat'
import { useAuth } from '../hooks/useAuth'

// Singleton adapter — stable across renders, safe to omit from hook deps
const rc = getRevenueCatAdapter()

interface RevenueCatContextType {
  offerings: PurchasesOfferings | null
  customerInfo: CustomerInfo | null
  isLoading: boolean
  purchasePackage: (aPackage: PurchasesPackage) => Promise<{ productIdentifier: string; customerInfo: CustomerInfo }>
  restorePurchases: () => Promise<CustomerInfo>
  refreshCustomerInfo: () => Promise<void>
  refreshOfferings: () => Promise<void>
}

export const RevenueCatContext = createContext<RevenueCatContextType | null>(null)

export function RevenueCatProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const [offerings, setOfferings] = useState<PurchasesOfferings | null>(null)
  const [customerInfo, setCustomerInfo] = useState<CustomerInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const initializedRef = useRef(false)

  const loadInitialData = useCallback(async () => {
    try {
      const [offeringsResult, customerInfoResult] = await Promise.allSettled([
        rc.getOfferings(),
        rc.getCustomerInfo(),
      ])
      if (offeringsResult.status === 'fulfilled') setOfferings(offeringsResult.value)
      if (customerInfoResult.status === 'fulfilled') setCustomerInfo(customerInfoResult.value)
    } catch (e) {
      if (__DEV__) console.error('[RevenueCat] Failed to load initial data:', e)
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Configure RevenueCat when user is available.
  //
  // Not gated on isNative: web sells the same catalog through Paddle, and the gate did two
  // kinds of damage there. It skipped configure, so no offering ever loaded; and because it
  // returned before loadInitialData, isLoading stayed true forever and the paywall sat on
  // its spinner rather than ever reporting that there was nothing to sell.
  useEffect(() => {
    if (!user?.id) return
    if (initializedRef.current) return
    initializedRef.current = true

    rc.configure(user.id)
    loadInitialData()
  }, [user?.id, loadInitialData])

  // Listen for customer info updates. The web adapter has no push channel and returns a
  // no-op unsubscribe, so this is harmless there — purchases refresh customerInfo directly.
  useEffect(() => {
    const unsubscribe = rc.addCustomerInfoUpdateListener((info) => {
      setCustomerInfo(info)
    })
    return () => { unsubscribe() }
  }, [])

  const purchasePackage = useCallback(async (aPackage: PurchasesPackage) => {
    return rc.purchasePackage(aPackage)
  }, [])

  const restorePurchases = useCallback(async () => {
    const info = await rc.restorePurchases()
    setCustomerInfo(info)
    return info
  }, [])

  const refreshCustomerInfo = useCallback(async () => {
    try {
      const info = await rc.getCustomerInfo()
      setCustomerInfo(info)
    } catch (e) {
      if (__DEV__) console.error('[RevenueCat] Failed to refresh customer info:', e)
    }
  }, [])

  const refreshOfferings = useCallback(async () => {
    try {
      const result = await rc.getOfferings()
      setOfferings(result)
    } catch (e) {
      if (__DEV__) console.error('[RevenueCat] Failed to refresh offerings:', e)
    }
  }, [])

  const value = useMemo(() => ({
    offerings,
    customerInfo,
    isLoading,
    purchasePackage,
    restorePurchases,
    refreshCustomerInfo,
    refreshOfferings,
  }), [offerings, customerInfo, isLoading, purchasePackage, restorePurchases, refreshCustomerInfo, refreshOfferings])

  return (
    <RevenueCatContext.Provider value={value}>
      {children}
    </RevenueCatContext.Provider>
  )
}
