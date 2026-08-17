export interface AuthCredentials {
  type: 'email' | 'phone'
  identifier: string
  code: string
}

export interface GoogleAuthPayload {
  accessToken: string
}

export interface AppleAuthPayload {
  identityToken: string
  email?: string
  name?: string
}

export interface RegisterPayload {
  email?: string
  name: string
  phoneNumber?: string
  preferredLanguage: string
  termsAndConditionsAccepted?: boolean
  aiAccepted?: boolean
  marketingAccepted?: boolean
  googleIdToken?: string
  googleAccessToken?: string
  appleIdentityToken?: string
  emailVerificationId?: string
  phoneVerificationId?: string
  /** Data-residency region, chosen during signup. It decides which regional database
   *  the profile is created in, so it cannot be changed later without migrating the
   *  data — which is why it is asked here and not during the voice onboarding. */
  dataRegion?: 'eu' | 'us'
}

export interface AuthTokens {
  accessToken: string
  refreshToken?: string
}

export interface AuthApi {
  requestVerificationCode(type: 'email' | 'phone', identifier: string): Promise<string>
  requestVerificationCodeWithIdentifier(type: 'email' | 'phone', identifier: string, event: string): Promise<string>
  loginWithCode(credentials: AuthCredentials): Promise<AuthTokens>
  register(data: RegisterPayload): Promise<AuthTokens>
  loginWithGoogle(payload: GoogleAuthPayload): Promise<AuthTokens>
  loginWithApple(payload: AppleAuthPayload): Promise<AuthTokens>
  /** Attaches an Apple ID to the CURRENT account. The only way in for "Hide My
   *  Email" users: their token carries a per-app relay address that can never
   *  match an account created with Google or by OTP, so signing in with Apple
   *  has nothing to match on until the credential is linked from here. */
  linkAppleAccount(identityToken: string): Promise<void>
  logout(): Promise<void>
  verifyCode(type: 'email' | 'phone', identifier: string, code: string): Promise<void>
  verifyAndUpdateIdentifier(type: 'email' | 'phone', identifier: string, verificationId: string): Promise<{ id: string; email?: string; phoneNumber?: string; name?: string; [key: string]: unknown }>
  /**
   * Returns the current access token for contexts that still need it directly
   * (e.g., the WebSocket URL). On native this reads secure storage; on web it
   * fetches from the BFF session endpoint authenticated by the HttpOnly cookie.
   */
  getSessionToken(): Promise<string | null>
}
