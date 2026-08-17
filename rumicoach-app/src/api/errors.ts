import i18n from '../i18n'

// Stable error codes returned by the backend in the `code` field. Mirror of
// internal/apierror in rumi-be — keep in sync when the backend adds a code.
export type BackendErrorCode =
  | 'INVALID_PAYLOAD'
  | 'INTERNAL_ERROR'
  | 'UNAUTHENTICATED'
  | 'WRONG_REGION'
  | 'ADMIN_REQUIRED'
  | 'TOO_MANY_REQUESTS'
  | 'IDENTIFIER_REQUIRED'
  | 'INVALID_CODE'
  | 'VERIFICATION_REQUIRED'
  | 'ACCOUNT_NOT_FOUND'
  | 'ACCOUNT_NOT_ACTIVE'
  | 'EMAIL_ALREADY_EXISTS'
  | 'PHONE_ALREADY_EXISTS'
  | 'ACCOUNT_ALREADY_EXISTS'
  | 'UNSUPPORTED_REGION'
  | 'UNSUPPORTED_SSO_PROVIDER'
  | 'GOOGLE_TOKEN_INVALID'
  | 'GOOGLE_EMAIL_MISSING'
  | 'GOOGLE_EMAIL_MISMATCH'
  | 'APPLE_TOKEN_INVALID'
  | 'SSO_EMAIL_NOT_VERIFIED'
  | 'APPLE_PRIVATE_EMAIL_UNLINKED'
  | 'APPLE_ALREADY_LINKED'
  | 'INVALID_REFRESH_TOKEN'
  | 'REFRESH_TOKEN_EXPIRED'
  | 'AUTH_TOKEN_MISSING'

// Each code maps to an i18n key (err_* in the locale files).
const CODE_TO_KEY: Record<BackendErrorCode, string> = {
  INVALID_PAYLOAD: 'err_invalid_payload',
  INTERNAL_ERROR: 'err_internal',
  UNAUTHENTICATED: 'err_unauthenticated',
  WRONG_REGION: 'err_wrong_region',
  ADMIN_REQUIRED: 'err_admin_required',
  TOO_MANY_REQUESTS: 'err_too_many_requests',
  IDENTIFIER_REQUIRED: 'err_identifier_required',
  INVALID_CODE: 'err_invalid_code',
  VERIFICATION_REQUIRED: 'err_verification_required',
  ACCOUNT_NOT_FOUND: 'err_account_not_found',
  ACCOUNT_NOT_ACTIVE: 'err_account_not_active',
  EMAIL_ALREADY_EXISTS: 'err_email_already_exists',
  PHONE_ALREADY_EXISTS: 'err_phone_already_exists',
  ACCOUNT_ALREADY_EXISTS: 'err_account_already_exists',
  UNSUPPORTED_REGION: 'err_unsupported_region',
  UNSUPPORTED_SSO_PROVIDER: 'err_unsupported_sso_provider',
  GOOGLE_TOKEN_INVALID: 'err_google_token_invalid',
  GOOGLE_EMAIL_MISSING: 'err_google_email_missing',
  GOOGLE_EMAIL_MISMATCH: 'err_google_email_mismatch',
  APPLE_TOKEN_INVALID: 'err_apple_token_invalid',
  SSO_EMAIL_NOT_VERIFIED: 'err_sso_email_not_verified',
  APPLE_PRIVATE_EMAIL_UNLINKED: 'err_apple_private_email_unlinked',
  APPLE_ALREADY_LINKED: 'err_apple_already_linked',
  INVALID_REFRESH_TOKEN: 'err_invalid_refresh_token',
  REFRESH_TOKEN_EXPIRED: 'err_refresh_token_expired',
  AUTH_TOKEN_MISSING: 'err_auth_token_missing',
}

interface ParsedError {
  code?: string
  message?: string
  status?: number
}

// parseApiError normalizes the various shapes an error can arrive in — an axios
// error (err.response.data), an already-parsed backend body ({ code, error }), or a
// plain Error — into { code, message, status }.
export function parseApiError(err: unknown): ParsedError {
  if (err && typeof err === 'object') {
    // axios error
    const anyErr = err as {
      response?: { status?: number; data?: { code?: string; error?: string } }
      code?: string
      error?: string
      message?: string
    }
    if (anyErr.response?.data) {
      return {
        code: anyErr.response.data.code,
        message: anyErr.response.data.error,
        status: anyErr.response.status,
      }
    }
    // already-parsed backend body
    if (anyErr.code || anyErr.error) {
      return { code: anyErr.code, message: anyErr.error }
    }
    if (anyErr.message) return { message: anyErr.message }
  }
  return {}
}

// messageForApiError returns a localized, customer-facing message for an error.
// It prefers the backend's code (localized copy); falls back to the backend's raw
// message, then to a provided fallback i18n key, then to a generic message.
export function messageForApiError(err: unknown, fallbackKey = 'err_generic'): string {
  const { code, message } = parseApiError(err)
  if (code && CODE_TO_KEY[code as BackendErrorCode]) {
    return i18n.t(CODE_TO_KEY[code as BackendErrorCode])
  }
  if (message) return message
  return i18n.t(fallbackKey)
}
