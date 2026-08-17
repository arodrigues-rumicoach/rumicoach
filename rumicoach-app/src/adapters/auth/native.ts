import { Platform } from 'react-native'
import { GoogleSignin } from '@react-native-google-signin/google-signin'
import * as AppleAuthentication from 'expo-apple-authentication'
import { Config } from '../../config'
import type { AuthAdapter, AppleSignInResult, GoogleSignInResult } from './types'

const nativeAuth: AuthAdapter = {
  configure: () => {
    if (__DEV__) console.log('[GOOGLE] configure: webClientId present=', !!Config.GOOGLE_CLIENT_ID_WEB)
    GoogleSignin.configure({ webClientId: Config.GOOGLE_CLIENT_ID_WEB })
  },
  signIn: async (): Promise<GoogleSignInResult> => {
    if (__DEV__) console.log('[GOOGLE] signIn: checking Play Services')
    await GoogleSignin.hasPlayServices()
    try {
      await GoogleSignin.signOut()
    } catch (e) {
      // no previous session to clear, continue
    }
    if (__DEV__) console.log('[GOOGLE] signIn: launching account picker')
    const result = await GoogleSignin.signIn()
    if (__DEV__) console.log('[GOOGLE] signIn: result type=', result.type)
    if (result.type === 'cancelled') {
      throw new Error('User cancelled')
    }
    if (__DEV__) {
      console.log('[GOOGLE] signIn: result.data.idToken present=', !!result.data?.idToken)
      console.log('[GOOGLE] signIn: result.data.user.email=', result.data?.user?.email)
    }
    if (__DEV__) console.log('[GOOGLE] signIn: calling getTokens()')
    const tokens = await GoogleSignin.getTokens()
    if (__DEV__) {
      console.log('[GOOGLE] signIn: tokens.accessToken present=', !!tokens.accessToken)
      console.log('[GOOGLE] signIn: tokens.idToken present=', !!tokens.idToken)
      if (tokens.accessToken) {
        console.log('[GOOGLE] signIn: accessToken prefix=', tokens.accessToken.slice(0, 16))
      }
    }
    const profile = result.data.user
    return {
      accessToken: tokens.accessToken,
      idToken: tokens.idToken,
      profile: {
        email: profile.email ?? undefined,
        name: profile.name ?? undefined,
      },
    }
  },
  signInWithApple: async (): Promise<AppleSignInResult> => {
    if (Platform.OS !== 'ios') {
      throw new Error('Apple Sign-In is only available on iOS')
    }
    if (__DEV__) console.log('[APPLE] signInWithApple: launching')
    const credential = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
    })
    if (__DEV__) console.log('[APPLE] signInWithApple: received identityToken present=', !!credential.identityToken)
    if (!credential.identityToken) {
      throw new Error('Apple Sign-In failed: no identity token')
    }
    return {
      identityToken: credential.identityToken,
      user: {
        email: credential.email ?? undefined,
        name: credential.fullName
          ? [credential.fullName.givenName, credential.fullName.familyName].filter(Boolean).join(' ')
          : undefined,
      },
    }
  },
}

export default nativeAuth
