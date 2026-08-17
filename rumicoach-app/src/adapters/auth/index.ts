import { isWeb } from '../platform'
import nativeAuth from './native'
import webAuth from './web'

import type { AuthAdapter } from './types'

export type { AppleSignInResult, GoogleSignInResult, AuthAdapter } from './types'

export const getAuthAdapter = (): AuthAdapter => (isWeb ? webAuth : nativeAuth)
