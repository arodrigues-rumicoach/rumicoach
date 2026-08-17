export interface GoogleSignInResult {
  accessToken: string
  idToken?: string
  profile?: { email?: string; name?: string }
}

export interface AppleSignInResult {
  identityToken: string
  user?: { email?: string; name?: string }
}

export interface AuthAdapter {
  configure(): void
  signIn(): Promise<GoogleSignInResult>
  signInWithApple(): Promise<AppleSignInResult>
}
