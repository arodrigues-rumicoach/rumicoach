export type NotificationHandler = (title: string, body: string) => void
export type BackgroundMessageHandler = (message: unknown) => Promise<void>

export interface NotificationsAdapter {
  requestPermission(): Promise<boolean>
  getToken(): Promise<string | null>
  registerToken(token: string): Promise<void>
  registerBackgroundHandler(handler: BackgroundMessageHandler): void
  registerForegroundHandler(handler: NotificationHandler): () => void
}
