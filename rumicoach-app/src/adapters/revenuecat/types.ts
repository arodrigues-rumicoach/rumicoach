import type { PurchasesPackage, CustomerInfo, PurchasesOffering, PurchasesOfferings } from 'react-native-purchases'

export interface RevenueCatAdapter {
  configure(appUserID: string): void
  getOfferings(): Promise<PurchasesOfferings>
  purchasePackage(aPackage: PurchasesPackage): Promise<{ productIdentifier: string; customerInfo: CustomerInfo }>
  restorePurchases(): Promise<CustomerInfo>
  getCustomerInfo(): Promise<CustomerInfo>
  logIn(appUserID: string): Promise<void>
  logOut(): Promise<void>
  addCustomerInfoUpdateListener(listener: (customerInfo: CustomerInfo) => void): () => boolean
}

export type { PurchasesPackage, CustomerInfo, PurchasesOffering, PurchasesOfferings }

// Web SDK types (used by web adapter)
export interface WebPackage {
  identifier: string
  packageType: string
  webBillingProduct: {
    identifier: string
    title: string
    price: number
    priceString: string
    currencyCode: string
  }
}

export interface WebCustomerInfo {
  entitlements: {
    active: Record<string, unknown>
  }
  activeSubscriptions: Set<string>
}

export interface WebOfferings {
  current: WebPackage | null
  all: Record<string, WebPackage>
}
