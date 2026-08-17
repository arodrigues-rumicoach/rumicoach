import authApi from './native'

import type { AuthApi } from './types'

export type { AuthCredentials, GoogleAuthPayload, RegisterPayload, AuthTokens, AuthApi } from './types'

// Single direct-backend implementation used on all platforms. Web talks to the
// auth backend directly (bearer tokens), same as native.
export const getAuthApi = (): AuthApi => authApi
