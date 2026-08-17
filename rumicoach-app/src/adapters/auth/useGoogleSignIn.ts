import { useCallback } from 'react'
import * as Google from 'expo-auth-session/providers/google'
import * as WebBrowser from 'expo-web-browser'
import { Config } from '../../config'
import { isWeb } from '../platform'
import { getAuthAdapter, type GoogleSignInResult } from './index'

WebBrowser.maybeCompleteAuthSession()

export function useGoogleSignIn(): () => Promise<GoogleSignInResult> {
  const nativeSignIn = useCallback(async () => getAuthAdapter().signIn(), [])

  const [, , promptAsync] = Google.useAuthRequest({
    clientId: Config.GOOGLE_CLIENT_ID_WEB,
    iosClientId: Config.GOOGLE_CLIENT_ID_IOS,
    androidClientId: Config.GOOGLE_CLIENT_ID_ANDROID,
    selectAccount: true,
  })

  const webSignIn = useCallback(async () => {
    const result = await promptAsync()

    if (result.type === 'success' && result.authentication?.accessToken) {
      return { accessToken: result.authentication.accessToken }
    }

    throw new Error('Google sign-in failed or was cancelled')
  }, [promptAsync])

  return isWeb ? webSignIn : nativeSignIn
}
