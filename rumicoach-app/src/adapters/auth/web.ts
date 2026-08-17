import type { AuthAdapter, AppleSignInResult, GoogleSignInResult } from './types'

const webAuth: AuthAdapter = {
  configure: () => {
    // Web Google Sign-In is configured via expo-auth-session in useGoogleSignIn.
  },
  signIn: async (): Promise<GoogleSignInResult> => {
    throw new Error('Google sign-in on web must use useGoogleSignIn()')
  },
  signInWithApple: async (): Promise<AppleSignInResult> => {
    throw new Error('Apple Sign-In is not available on web')
  },
}

export default webAuth
