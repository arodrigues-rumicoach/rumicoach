// Package apierror standardizes JSON error responses. Every error carries a stable
// machine-readable `code` (an enum the frontend switches on to show a localized
// message) alongside a human-readable `error` string kept for logs and as a fallback.
package apierror

import (
	"encoding/json"
	"net/http"
)

// Code is a stable, machine-readable error identifier. Values are contract: the
// mobile/web clients map them to localized copy, so never repurpose an existing code
// — add a new one instead.
type Code string

const (
	// Generic / transport
	CodeInvalidPayload  Code = "INVALID_PAYLOAD"   // 400 malformed or unparseable body
	CodeInternalError   Code = "INTERNAL_ERROR"    // 500 unexpected server failure
	CodeUnauthenticated Code = "UNAUTHENTICATED"   // 401 missing/invalid access token
	CodeWrongRegion     Code = "WRONG_REGION"      // 403 token's data region != this plane
	CodeAdminRequired   Code = "ADMIN_REQUIRED"    // 403 admin privileges required
	CodeTooManyRequests Code = "TOO_MANY_REQUESTS" // 429 rate limited

	// Identifier / verification
	CodeIdentifierRequired   Code = "IDENTIFIER_REQUIRED"   // 400 email or phone required
	CodeInvalidCode          Code = "INVALID_CODE"          // 400 wrong/expired verification code
	CodeVerificationRequired Code = "VERIFICATION_REQUIRED" // 400 no valid verification for this signup

	// Account state
	CodeAccountNotFound      Code = "ACCOUNT_NOT_FOUND"      // 403 no account (join waitlist)
	CodeAccountNotActive     Code = "ACCOUNT_NOT_ACTIVE"     // 403 account exists but on waitlist
	CodeEmailAlreadyExists   Code = "EMAIL_ALREADY_EXISTS"   // 400 email already registered
	CodePhoneAlreadyExists   Code = "PHONE_ALREADY_EXISTS"   // 400 phone already registered
	CodeAccountAlreadyExists Code = "ACCOUNT_ALREADY_EXISTS" // 400 google account already registered
	// CodeIntegrationAlreadyActive — 409, the user already has this channel connected.
	// Issuing a fresh link code would tear down the working connection, so re-linking
	// requires an explicit disconnect first.
	CodeIntegrationAlreadyActive Code = "INTEGRATION_ALREADY_ACTIVE"
	CodeUnsupportedRegion        Code = "UNSUPPORTED_REGION" // 400 unsupported data region

	// SSO / Google
	CodeUnsupportedSSOProvider Code = "UNSUPPORTED_SSO_PROVIDER" // 400
	CodeGoogleTokenInvalid     Code = "GOOGLE_TOKEN_INVALID"     // token failed validation
	CodeGoogleEmailMissing     Code = "GOOGLE_EMAIL_MISSING"     // 400 token had no email
	CodeGoogleEmailMismatch    Code = "GOOGLE_EMAIL_MISMATCH"    // 400 token email != requested

	// CodeSSOEmailNotVerified — 400/403, the provider ITSELF reports the token's email
	// as unverified. Such an address may neither create an account nor be matched
	// against one: an identity provider that lets anyone claim an arbitrary address
	// would otherwise be a way to take over someone else's account by email.
	CodeSSOEmailNotVerified Code = "SSO_EMAIL_NOT_VERIFIED"

	// SSO / Apple
	CodeAppleTokenInvalid  Code = "APPLE_TOKEN_INVALID"  // identity token failed validation
	CodeAppleEmailMissing  Code = "APPLE_EMAIL_MISSING"  // 400 token had no email
	CodeAppleEmailMismatch Code = "APPLE_EMAIL_MISMATCH" // 400 token email != requested
	// CodeApplePrivateEmailUnlinked — 403, a "Hide My Email" token whose Apple ID is on
	// no account. The relay address is per-app, so it can never equal the email of an
	// account created with another provider and there is nothing to match on: the user
	// must sign in with their original method and link Apple from the account.
	CodeApplePrivateEmailUnlinked Code = "APPLE_PRIVATE_EMAIL_UNLINKED"
	// CodeAppleAlreadyLinked — 400, the account already carries a DIFFERENT Apple ID.
	// Overwriting it would silently revoke a credential that still works.
	CodeAppleAlreadyLinked Code = "APPLE_ALREADY_LINKED"

	// Refresh
	CodeInvalidRefreshToken Code = "INVALID_REFRESH_TOKEN" // 401
	CodeRefreshTokenExpired Code = "REFRESH_TOKEN_EXPIRED" // 401

	// Auth transport (WebSocket)
	CodeAuthTokenMissing Code = "AUTH_TOKEN_MISSING" // 401 no token on the connection

	// Balance
	CodeInsufficientBalance Code = "INSUFFICIENT_BALANCE" // 402 no session minutes left
)

// response is the JSON body written for every error.
type response struct {
	Error string `json:"error"`
	Code  Code   `json:"code"`
}

// Write emits a JSON error with the given HTTP status, stable code, and human message.
func Write(w http.ResponseWriter, status int, code Code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{Error: message, Code: code})
}
