export interface FirebaseAdapter {
  logEvent(name: string, params?: Record<string, unknown>): Promise<void>
  setUserId(userId: string | null): Promise<void>
  trackScreenView(screenName: string, screenClass?: string): Promise<void>
  setCrashlyticsUserId(userId: string | null): void
  logCrashlyticsError(error: Error): void
}
