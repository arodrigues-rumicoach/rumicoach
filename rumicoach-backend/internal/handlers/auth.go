package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/internal/services/regional"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"google.golang.org/api/idtoken"
)

// tokenResponse builds the auth response, telling the client which regional API
// (eu.rumi.coach / us.rumi.coach) to use for all non-auth calls.
func tokenResponse(identity *models.Identity, access, refreshToken string) api.TokenResponse {
	tType := "bearer"
	resp := api.TokenResponse{
		AccessToken:  &access,
		TokenType:    &tType,
		RefreshToken: &refreshToken,
	}

	region := api.TokenResponseDataRegion(identity.ResidencyRegion())
	resp.DataRegion = &region
	if baseURL := config.AppConfig.DataPlaneURL(identity.ResidencyRegion()); baseURL != "" {
		resp.ApiBaseUrl = &baseURL
	}
	return resp
}

// issueTokens creates the access token and rotates the refresh token for an identity,
// persisting the new refresh state on the auth database.
func issueTokens(identity *models.Identity) (api.TokenResponse, error) {
	displayName := ""
	if identity.Name != nil {
		displayName = *identity.Name
	}
	email := ""
	if identity.Email != nil {
		email = *identity.Email
	}

	access, err := auth.CreateAccessToken(identity.ID, email, displayName, identity.IsActive,
		identity.IsEmailVerified, identity.IsPhonenumberVerified, identity.IsAdmin, identity.ResidencyRegion())
	if err != nil {
		return api.TokenResponse{}, err
	}

	rfBytes := make([]byte, 48)
	if _, err := rand.Read(rfBytes); err != nil {
		return api.TokenResponse{}, err
	}
	refresh := base64.URLEncoding.EncodeToString(rfBytes)

	hash, err := auth.GetPasswordHash(refresh)
	if err != nil {
		return api.TokenResponse{}, err
	}
	expr := time.Now().Add(time.Duration(config.AppConfig.RefreshTokenExpire) * 24 * time.Hour)
	now := time.Now()

	identity.RefreshTokenHash = &hash
	identity.RefreshTokenExpiresAt = &expr
	identity.LastOnlineAt = &now

	if err := database.Auth().Save(identity).Error; err != nil {
		return api.TokenResponse{}, err
	}

	// Sync LastOnlineAt and LastPlatform to regional database (models.User)
	userUpdates := map[string]interface{}{
		"last_online_at": now,
	}
	if identity.Platform != nil && *identity.Platform != "" {
		userUpdates["last_platform"] = *identity.Platform
	}
	database.DB.Model(&models.User{}).Where("id = ?", identity.ID).Updates(userUpdates)

	return tokenResponse(identity, access, identity.ID+"."+refresh), nil
}

// ssoToken is the verified content of a provider identity token (Google or Apple).
// Only what is inside the token lands here — a client-supplied email travelling
// alongside it proves nothing and must never reach an account lookup.
type ssoToken struct {
	// subject is the provider's stable user id (the `sub` claim). It is the primary
	// account key: unlike the email, it never changes and is never re-issued.
	subject string
	email   string
	// emailVerified is the provider's own assertion about email: true, false, or nil
	// when the token carried no such claim. Absence is treated as verified on
	// purpose — Google and Apple always send it, so a missing claim means the payload
	// changed shape, and locking every user out of their account is a far worse
	// failure than the attack this gate exists for, which needs an explicit false.
	emailVerified *bool
	// privateRelay marks Apple's "Hide My Email" addresses. Nothing is refused
	// because of it (the relay IS the account's email when the account was created
	// with Apple); it exists so a failed lookup can say which of the two dead ends
	// the user hit.
	privateRelay bool
}

// emailIsVerified reports whether the provider vouches for the token's email.
func (t ssoToken) emailIsVerified() bool {
	return t.emailVerified == nil || *t.emailVerified
}

// linkableEmail returns the email that may be used to find (and then attach this
// credential to) an EXISTING account, or "" when the token's email must not be
// trusted for that. Account lookups by email go through this, never through
// t.email directly.
func (t ssoToken) linkableEmail() string {
	if t.email == "" || !t.emailIsVerified() {
		return ""
	}
	return t.email
}

// claimBool reads a JWT claim that identity providers send as either a JSON bool
// or the string "true"/"false" — Apple does both, depending on the claim and the
// flow. A missing claim returns nil, which callers must distinguish from false.
func claimBool(v any) *bool {
	switch t := v.(type) {
	case bool:
		return &t
	case string:
		b := t == "true"
		return &b
	}
	return nil
}

// linkProviderID attaches an SSO credential to an identity with a single-column
// update. A full-row Save would write back every field of the struct we happened
// to read, including a refresh-token hash a concurrent login may have rotated in
// the meantime.
func linkProviderID(identityID, column, providerID string) error {
	return database.Auth().Model(&models.Identity{}).
		Where("id = ?", identityID).
		Update(column, providerID).Error
}

// normalizeEmail canonicalizes an email for storage and comparison. Every email
// entering the auth layer — client-typed or SSO-token-verified — goes through
// this, and every email lookup compares LOWER(email) so legacy mixed-case rows
// still match. Without this, "Mando@x.com" and "mando@x.com" passed every
// duplicate check and produced two accounts for one mailbox (QA-found, via
// Apple + Google signups).
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// residencyRegion validates the region a signup request asked for. Empty defaults
// to "eu"; anything else must be a supported region.
func residencyRegion(requested *api.WaitlistRequestDataRegion) (string, error) {
	if requested == nil || *requested == "" {
		return "eu", nil
	}
	if !requested.Valid() {
		return "", fmt.Errorf("unsupported data region %q", string(*requested))
	}
	return string(*requested), nil
}

// RequestVerification implements api.ServerInterface
func (s *Server) RequestVerification(w http.ResponseWriter, r *http.Request) {
	var req api.RequestVerificationJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid payload")
		return
	}

	identifier := ""
	if req.Type == api.VerificationRequestTypeEmail && req.Email != nil {
		identifier = normalizeEmail(*req.Email)
		req.Email = &identifier
	} else if req.Type == api.VerificationRequestTypePhone && req.PhoneNumber != nil {
		identifier = *req.PhoneNumber
	}

	if identifier == "" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeIdentifierRequired, "Email or Phone Number is required")
		return
	}

	// Account-enumeration protection: the response is identical whether or not the
	// identifier already has an account. We only skip issuing a code when the commitment
	// can't proceed (signup for an existing account, login for a missing one), but the
	// caller can't tell that apart from a normal success. The real reason is logged
	// server-side only.
	var identity models.Identity
	var exists bool
	if req.Type == api.VerificationRequestTypeEmail && req.Email != nil {
		exists = database.Auth().Where("LOWER(email) = ?", *req.Email).First(&identity).Error == nil
	} else if req.Type == api.VerificationRequestTypePhone && req.PhoneNumber != nil {
		exists = database.Auth().Where("phone_number = ?", *req.PhoneNumber).First(&identity).Error == nil
	}

	event := ""
	if req.Event != nil {
		event = *req.Event
	}
	// A code is issued unless this is a login for an account that doesn't exist
	// (nothing to log into — the response stays identical so the caller can't tell).
	// Signup for an EXISTING account deliberately still sends a code: the "account
	// already exists" error is only revealed at /verify, after the caller has proven
	// ownership of the mailbox/number by entering it (see VerifyCode).
	shouldIssue := event != "login" || exists

	msg := "If the account can receive a code, one has been sent"
	verified := false
	resp := api.VerificationResponse{Message: &msg, Verified: &verified}

	if shouldIssue {
		code, verificationID, err := services.GlobalVerificationService.CreateVerificationCode(identifier, string(req.Type))
		switch {
		case errors.Is(err, services.ErrTooManyCodeRequests):
			// Generic message (don't reveal the identifier is being targeted) but a 429
			// with a code so a well-behaved client can back off.
			apierror.Write(w, http.StatusTooManyRequests, apierror.CodeTooManyRequests,
				"Too many requests. Please wait a moment and try again.")
			return
		case err != nil:
			apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Failed to create verification code")
			return
		}

		if req.Type == api.VerificationRequestTypeEmail && config.AppConfig.ReviewAccountCode(identifier) != "" {
			// Review/demo account: the code is fixed and pre-shared with the reviewer,
			// and the mailbox may not exist — issue the code but skip delivery.
			s.logger.Info("issued fixed verification code for review account")
		} else if req.Type == api.VerificationRequestTypeEmail {
			go func() {
				_ = notification.GlobalNotificationService.SendVerificationEmail(identifier, code, "en-US")
			}()
		} else {
			go func() {
				_ = notification.GlobalNotificationService.SendVerificationSMS(identifier, code, "en-US")
			}()
		}
		// The verification ID only serves the signup flow (JoinWaitlist/RegisterUser).
		// Login never uses it — and echoing it only when a code was actually issued
		// would reveal account existence, since login for a missing account issues
		// nothing. Omitting it for login keeps both responses byte-identical.
		if event != "login" {
			resp.VerificationId = &verificationID
		}
	} else {
		s.logger.Info("verification code not issued (event/account-state mismatch)",
			zap.String("event", event), zap.Bool("account_exists", exists))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// VerifyCode implements api.ServerInterface
func (s *Server) VerifyCode(w http.ResponseWriter, r *http.Request) {
	var req api.VerifyCodeJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid payload")
		return
	}

	identifier := ""
	if req.Type == api.VerificationCheckTypeEmail && req.Email != nil {
		identifier = normalizeEmail(*req.Email)
	} else if req.Type == api.VerificationCheckTypePhone && req.PhoneNumber != nil {
		identifier = *req.PhoneNumber
	}

	isValid, _ := services.GlobalVerificationService.VerifyCode(identifier, string(req.Type), req.Code)
	if !isValid {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidCode, "Invalid or expired code")
		return
	}

	// This endpoint serves the signup flow. Only now — after the caller proved
	// ownership of the identifier by entering the code it received — reveal whether
	// an account already exists. Revealing it any earlier (at request time, or on an
	// invalid code) would let anyone probe which emails/phones are registered.
	var existing models.Identity
	if req.Type == api.VerificationCheckTypeEmail {
		if database.Auth().Where("LOWER(email) = ?", identifier).First(&existing).Error == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeEmailAlreadyExists, "An account with this email already exists. Please log in instead.")
			return
		}
	} else if req.Type == api.VerificationCheckTypePhone {
		if database.Auth().Where("phone_number = ?", identifier).First(&existing).Error == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodePhoneAlreadyExists, "An account with this phone number already exists. Please log in instead.")
			return
		}
	}

	msg := "Successfully verified"
	verified := true
	resp := api.VerificationResponse{
		Message:  &msg,
		Verified: &verified,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// VerifySSO implements api.ServerInterface
func (s *Server) VerifySSO(w http.ResponseWriter, r *http.Request) {
	var req api.SSOVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}

	// This is the signup screen's pre-check: it answers "can this credential create a
	// new account?". EMAIL_ALREADY_EXISTS / ACCOUNT_ALREADY_EXISTS are not dead ends —
	// they mean the caller should stop signing up and log in with the same token
	// instead (/auth/google, /auth/apple), which links the credential to the account
	// it found. The status codes are kept as they are because shipped app builds
	// depend on them.
	switch req.Type {
	case api.Google:
		tok, status, err := s.validateGoogleToken(r.Context(), req.IdToken, req.AccessToken)
		if err != nil {
			apierror.Write(w, status, apierror.CodeGoogleTokenInvalid, err.Error())
			return
		}
		if tok.email != "" && !tok.emailIsVerified() {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeSSOEmailNotVerified,
				"Google reports this email as unverified. Verify it with Google first.")
			return
		}
		var identity models.Identity
		if err := database.Auth().Where("LOWER(email) = ?", normalizeEmail(tok.email)).First(&identity).Error; err == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeEmailAlreadyExists, "Email already registered")
			return
		}

	case api.Apple:
		// The app sends the identity token in both fields; accept either.
		token := ""
		if req.IdToken != nil {
			token = *req.IdToken
		}
		if token == "" && req.AccessToken != nil {
			token = *req.AccessToken
		}
		tok, status, err := s.validateAppleToken(token)
		if err != nil {
			apierror.Write(w, status, apierror.CodeAppleTokenInvalid, err.Error())
			return
		}
		if tok.email != "" && !tok.emailIsVerified() {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeSSOEmailNotVerified,
				"Apple reports this email as unverified.")
			return
		}
		// Existence is keyed on the stable Apple subject first: with "Hide My
		// Email" the token email is a private relay that may not match the
		// account's stored address.
		var identity models.Identity
		if err := database.Auth().Where("apple_id = ?", tok.subject).First(&identity).Error; err == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeAccountAlreadyExists, "Apple account already registered")
			return
		}
		if tok.email != "" {
			if err := database.Auth().Where("LOWER(email) = ?", normalizeEmail(tok.email)).First(&identity).Error; err == nil {
				apierror.Write(w, http.StatusBadRequest, apierror.CodeEmailAlreadyExists, "Email already registered")
				return
			}
		}

	default:
		apierror.Write(w, http.StatusBadRequest, apierror.CodeUnsupportedSSOProvider, "Unsupported SSO provider")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// LoginWithCode implements api.ServerInterface
func (s *Server) LoginWithCode(w http.ResponseWriter, r *http.Request) {
	var req api.LoginWithCodeJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}

	if req.Type == api.LoginWithCodeRequestTypeEmail {
		req.Identifier = normalizeEmail(req.Identifier)
	}

	isValid, _ := services.GlobalVerificationService.VerifyCode(req.Identifier, string(req.Type), req.Code)
	if !isValid {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidCode, "Invalid or expired code")
		return
	}

	var identity models.Identity
	query := database.Auth().Where("LOWER(email) = ?", req.Identifier)
	if req.Type == api.LoginWithCodeRequestTypePhone {
		query = database.Auth().Where("phone_number = ?", req.Identifier)
	}

	if err := query.First(&identity).Error; err != nil {
		apierror.Write(w, http.StatusForbidden, apierror.CodeAccountNotFound, "User not found. Please join the waiting list first.")
		return
	}

	if !identity.IsActive {
		apierror.Write(w, http.StatusForbidden, apierror.CodeAccountNotActive, "Account is not active. You are on the waiting list.")
		return
	}

	resp, err := issueTokens(&identity)
	if err != nil {
		s.logger.Error("Failed to issue tokens", zap.Error(err), zap.String("user_id", identity.ID))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// validateGoogleToken validates a Google ID token or access token and returns its
// verified subject, email, and email-verification flag (see ssoToken), plus an
// HTTP status and error on failure.
func (s *Server) validateGoogleToken(ctx context.Context, idToken *string, accessToken *string) (ssoToken, int, error) {
	configIDs := strings.Split(config.AppConfig.GoogleClientIDs, ",")
	var tok ssoToken

	// Determine if we should attempt ID Token validation (JWT format: 3 parts, 2 dots)
	isIDTokenJWT := idToken != nil && *idToken != "" && strings.Count(*idToken, ".") == 2

	if isIDTokenJWT {
		if len(configIDs) == 0 || configIDs[0] == "" {
			return tok, http.StatusInternalServerError, fmt.Errorf("Google Client IDs not configured")
		}
		var payload *idtoken.Payload
		var err error
		for _, clientID := range configIDs {
			clientID = strings.TrimSpace(clientID)
			if clientID == "" {
				continue
			}
			s.logger.Info("Attempting Google ID Token validation", zap.String("client_id", clientID))
			payload, err = idtoken.Validate(ctx, *idToken, clientID)
			if err == nil {
				s.logger.Info("Google ID Token validation succeeded", zap.String("client_id", clientID))
				break
			}
			s.logger.Warn("Google ID Token validation failed for client", zap.String("client_id", clientID), zap.Error(err))
		}
		if err != nil {
			// A rejected Google token is the normal outcome of an expired or forged credential —
			// the client is told, and there is nothing for us to fix.
			s.logger.Info("Google token validation failed", zap.Error(err))
			return tok, http.StatusUnauthorized, fmt.Errorf("Invalid Google Token")
		}
		email, ok := payload.Claims["email"].(string)
		if !ok || email == "" {
			return tok, http.StatusUnauthorized, fmt.Errorf("Incomplete user info from Google")
		}
		tok.subject = payload.Subject
		tok.email = email
		tok.emailVerified = claimBool(payload.Claims["email_verified"])
	} else {
		// Fallback: If idToken is present but not a JWT, it is likely an access token.
		tokenToUse := accessToken
		if (tokenToUse == nil || *tokenToUse == "") && idToken != nil && *idToken != "" {
			tokenToUse = idToken
		}

		if tokenToUse != nil && *tokenToUse != "" {
			reqURI := "https://www.googleapis.com/oauth2/v3/userinfo"
			clientReq, err := http.NewRequestWithContext(ctx, "GET", reqURI, nil)
			if err != nil {
				return tok, http.StatusInternalServerError, fmt.Errorf("Internal server error")
			}
			clientReq.Header.Set("Authorization", "Bearer "+*tokenToUse)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(clientReq)
			if err != nil {
				return tok, http.StatusInternalServerError, fmt.Errorf("Failed to contact Google")
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return tok, http.StatusUnauthorized, fmt.Errorf("Invalid access token")
			}

			var userInfo map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
				return tok, http.StatusInternalServerError, fmt.Errorf("Invalid response from Google")
			}

			sub, ok1 := userInfo["sub"].(string)
			em, ok2 := userInfo["email"].(string)
			if !ok1 || !ok2 {
				return tok, http.StatusUnauthorized, fmt.Errorf("Incomplete user info from Google")
			}

			tok.subject = sub
			tok.email = em
			tok.emailVerified = claimBool(userInfo["email_verified"])
		} else {
			return tok, http.StatusBadRequest, fmt.Errorf("id_token or access_token required")
		}
	}

	if !tok.emailIsVerified() {
		// Google sends email_verified on every token carrying an email, so this is
		// worth seeing: it is either a Workspace account on an unverified domain or a
		// payload we no longer understand. Either way the address stops being usable
		// to find an account.
		s.logger.Warn("Google token reports an unverified email; it will not be used to match an account")
	}
	return tok, http.StatusOK, nil
}

// GoogleLogin implements api.ServerInterface
func (s *Server) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	s.logger.Info("Received GoogleLogin request",
		zap.Int("body_length", len(bodyBytes)))

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req api.GoogleLoginJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// The body is NOT logged. On this endpoint it carries a Google ID or access token —
		// a live credential — and a malformed body is no reason to write one into a log
		// that is retained, searchable, and readable by anyone with project access. The
		// decode error names the offending position, which is what actually helps.
		s.logger.Info("could not decode GoogleLogin request",
			zap.Error(err), zap.Int("body_length", len(bodyBytes)))
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}

	// Both credentials are reported because either one is valid: validateGoogleToken accepts
	// an ID token or an access token. Logging only id_token_present made a perfectly good
	// login read as "the client sent no token", which is the kind of line that sends someone
	// chasing the wrong thing for an hour during an incident.
	s.logger.Info("Successfully decoded GoogleLogin request",
		zap.Bool("id_token_present", req.IdToken != nil),
		zap.Bool("access_token_present", req.AccessToken != nil))

	tok, status, err := s.validateGoogleToken(r.Context(), req.IdToken, req.AccessToken)
	if err != nil {
		apierror.Write(w, status, apierror.CodeGoogleTokenInvalid, err.Error())
		return
	}

	var identity models.Identity
	if err := database.Auth().Where("google_id = ?", tok.subject).First(&identity).Error; err != nil {
		// Fallback for an account this Google credential is not stamped on yet —
		// created by OTP, by Apple, or before Google sign-in existed. Only an email
		// Google itself vouches for may find it (see ssoToken.linkableEmail).
		linkEmail := tok.linkableEmail()
		if linkEmail == "" {
			apierror.Write(w, http.StatusForbidden, apierror.CodeSSOEmailNotVerified,
				"Google reports this email as unverified, so it cannot be matched to an account.")
			return
		}
		if errEmail := database.Auth().Where("LOWER(email) = ?", normalizeEmail(linkEmail)).First(&identity).Error; errEmail == nil {
			if err := linkProviderID(identity.ID, "google_id", tok.subject); err != nil {
				s.logger.Error("failed to link Google ID", zap.Error(err), zap.String("user_id", identity.ID))
				apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
				return
			}
			identity.GoogleID = &tok.subject
		} else {
			apierror.Write(w, http.StatusForbidden, apierror.CodeAccountNotFound, "No account found. Please join the waiting list first.")
			return
		}
	}

	if !identity.IsActive {
		apierror.Write(w, http.StatusForbidden, apierror.CodeAccountNotActive, "Account is not active.")
		return
	}

	if platform := r.Header.Get("X-Platform"); platform != "" {
		identity.Platform = &platform
	}

	resp, err := issueTokens(&identity)
	if err != nil {
		s.logger.Error("Failed to issue tokens", zap.Error(err), zap.String("user_id", identity.ID))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RefreshToken implements api.ServerInterface
func (s *Server) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req api.RefreshTokenJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}

	parts := strings.SplitN(req.RefreshToken, ".", 2)
	if len(parts) != 2 {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeInvalidRefreshToken, "Invalid refresh token format")
		return
	}

	userID := parts[0]
	rawToken := parts[1]

	var identity models.Identity
	if err := database.Auth().Where("id = ?", userID).First(&identity).Error; err != nil {
		// Info, not Error: a refresh token pointing at an identity that no longer exists is
		// routine and client-driven — a token left in storage after the account was deleted,
		// a reset local database, a credential from an older scheme. The 401 IS the correct
		// answer and the client re-authenticates. Logging it as an error buries the ones
		// that are real, and the development logger attaches a stack trace to every one.
		s.logger.Info("refresh token names an unknown identity; asking the client to sign in again",
			zap.String("user_id", userID))
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeInvalidRefreshToken, "Invalid refresh token")
		return
	}

	if identity.RefreshTokenHash == nil || identity.RefreshTokenExpiresAt == nil {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeInvalidRefreshToken, "No refresh token exists")
		return
	}

	if time.Now().After(*identity.RefreshTokenExpiresAt) {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeRefreshTokenExpired, "Refresh token expired")
		return
	}

	if !auth.VerifyPassword(rawToken, *identity.RefreshTokenHash) {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeInvalidRefreshToken, "Invalid refresh token")
		return
	}

	// Extract platform if provided
	if platform := ExtractPlatform(r); platform != "" {
		identity.Platform = &platform
	}

	// Rotate: issueTokens generates and persists the replacement refresh token.
	resp, err := issueTokens(&identity)
	if err != nil {
		s.logger.Error("Failed to issue tokens", zap.Error(err), zap.String("user_id", identity.ID))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// signupCredentials records how a signup request proved ownership of its
// identifier: an SSO token (Google or Apple — the verified token email is
// authoritative and counts as email verification) or a completed OTP verification.
type signupCredentials struct {
	googleID      string
	appleID       string
	emailVerified bool
	phoneVerified bool
}

// verifySignupCredentials validates the credential on a signup request (shared by
// JoinWaitlist and RegisterUser) and normalizes req.Email to the verified SSO
// email when the client omitted it. A non-zero status means the request must be
// rejected with (status, code, msg).
func (s *Server) verifySignupCredentials(ctx context.Context, req *api.WaitlistRequest) (signupCredentials, int, apierror.Code, string) {
	var creds signupCredentials

	var emailVal string
	if req.Email != nil {
		emailVal = normalizeEmail(*req.Email)
		req.Email = &emailVal
	}
	var phoneVal string
	if req.PhoneNumber != nil {
		phoneVal = *req.PhoneNumber
	}

	switch {
	case (req.GoogleIdToken != nil && *req.GoogleIdToken != "") || (req.GoogleAccessToken != nil && *req.GoogleAccessToken != ""):
		tok, status, err := s.validateGoogleToken(ctx, req.GoogleIdToken, req.GoogleAccessToken)
		if err != nil {
			return creds, status, apierror.CodeGoogleTokenInvalid, err.Error()
		}
		gID := tok.subject
		googleEmail := normalizeEmail(tok.email)
		if googleEmail == "" {
			return creds, http.StatusBadRequest, apierror.CodeGoogleEmailMissing, "Google token did not include an email"
		}
		// An account keyed on an address the provider does not vouch for is the setup
		// half of an account takeover: it squats on the real owner's email until they
		// arrive. It also cannot honestly be recorded as email-verified.
		if !tok.emailIsVerified() {
			return creds, http.StatusBadRequest, apierror.CodeSSOEmailNotVerified,
				"Google reports this email as unverified. Verify it with Google first."
		}
		// SSO signup: the client may omit the email; the verified Google email is
		// authoritative. When both are present they must agree.
		if emailVal == "" {
			req.Email = &googleEmail
		} else if !strings.EqualFold(googleEmail, emailVal) {
			return creds, http.StatusBadRequest, apierror.CodeGoogleEmailMismatch, "Google email does not match requested email"
		}
		creds.googleID = gID
		creds.emailVerified = true

	case req.AppleIdentityToken != nil && *req.AppleIdentityToken != "":
		tok, status, err := s.validateAppleToken(*req.AppleIdentityToken)
		if err != nil {
			return creds, status, apierror.CodeAppleTokenInvalid, err.Error()
		}
		aID := tok.subject
		appleEmail := normalizeEmail(tok.email)
		if appleEmail == "" {
			// Apple puts the verified email (real or private relay) in every identity
			// token issued with the email scope; without one the account would be
			// unreachable, and a client-supplied address is unverified.
			return creds, http.StatusBadRequest, apierror.CodeAppleEmailMissing, "Apple token did not include an email"
		}
		if !tok.emailIsVerified() {
			return creds, http.StatusBadRequest, apierror.CodeSSOEmailNotVerified,
				"Apple reports this email as unverified."
		}
		if emailVal == "" {
			req.Email = &appleEmail
		} else if !strings.EqualFold(appleEmail, emailVal) {
			return creds, http.StatusBadRequest, apierror.CodeAppleEmailMismatch, "Apple email does not match requested email"
		}
		creds.appleID = aID
		creds.emailVerified = true

	case emailVal == "" && phoneVal == "":
		return creds, http.StatusBadRequest, apierror.CodeIdentifierRequired, "Email or Phone Number is required"

	default:
		var emailVerificationID string
		if req.EmailVerificationId != nil {
			emailVerificationID = *req.EmailVerificationId
		}
		var phoneVerificationID string
		if req.PhoneVerificationId != nil {
			phoneVerificationID = *req.PhoneVerificationId
		}

		if emailVal != "" && emailVerificationID != "" && services.GlobalVerificationService.ValidateVerificationID(emailVerificationID, emailVal, "email") {
			creds.emailVerified = true
		} else if phoneVal != "" && phoneVerificationID != "" && services.GlobalVerificationService.ValidateVerificationID(phoneVerificationID, phoneVal, "phone") {
			creds.phoneVerified = true
		} else {
			return creds, http.StatusBadRequest, apierror.CodeVerificationRequired, "Verification required or invalid"
		}
	}

	return creds, 0, "", ""
}

// JoinWaitlist implements api.ServerInterface
func (s *Server) JoinWaitlist(w http.ResponseWriter, r *http.Request) {
	var req api.JoinWaitlistJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid payload format")
		return
	}

	creds, status, code, errMsg := s.verifySignupCredentials(r.Context(), &req)
	if status != 0 {
		apierror.Write(w, status, code, errMsg)
		return
	}

	// Read the identifiers after credential verification: an SSO signup may have
	// filled req.Email with the verified token email.
	var emailVal string
	if req.Email != nil {
		emailVal = *req.Email
	}
	var phoneVal string
	if req.PhoneNumber != nil {
		phoneVal = *req.PhoneNumber
	}

	identity, status, code, errMsg := s.createAccount(r.Context(), &req, creds, false)
	if status != 0 {
		apierror.Write(w, status, code, errMsg)
		return
	}

	if emailVal != "" {
		go func() {
			_ = notification.GlobalNotificationService.SendWaitlistJoinedEmail(emailVal, req.Name, "en-US")
		}()
	} else if phoneVal != "" {
		go func() {
			_ = notification.GlobalNotificationService.SendWaitlistJoinedSMS(phoneVal, req.Name, "en-US")
		}()
	}

	var dob *openapi_types.Date
	if req.DateOfBirth != nil {
		dob = &openapi_types.Date{Time: req.DateOfBirth.Time}
	}

	theme := "waterfall"
	userRegion := api.UserDataRegion(identity.ResidencyRegion())

	apiUser := api.User{
		Id:                           &identity.ID,
		Email:                        identity.Email,
		PhoneNumber:                  identity.PhoneNumber,
		Name:                         identity.Name,
		DateOfBirth:                  dob,
		Gender:                       req.Gender,
		Country:                      req.Country,
		Region:                       req.Region,
		DataRegion:                   &userRegion,
		PreferredLanguage:            req.PreferredLanguage,
		IsActive:                     &identity.IsActive,
		Theme:                        &theme,
		TermsAndConditionsAcceptedAt: identity.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          identity.MarketingAcceptedAt,
		AiAcceptedAt:                 identity.AIAcceptedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUser)
}

// RegisterUser implements api.ServerInterface
func (s *Server) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var req api.RegisterUserJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid payload format")
		return
	}

	creds, status, code, errMsg := s.verifySignupCredentials(r.Context(), &req)
	if status != 0 {
		apierror.Write(w, status, code, errMsg)
		return
	}

	// Read the identifiers after credential verification: an SSO signup may have
	// filled req.Email with the verified token email.
	var emailVal string
	if req.Email != nil {
		emailVal = *req.Email
	}
	var phoneVal string
	if req.PhoneNumber != nil {
		phoneVal = *req.PhoneNumber
	}

	identity, status, code, errMsg := s.createAccount(r.Context(), &req, creds, true)
	if status != 0 {
		apierror.Write(w, status, code, errMsg)
		return
	}

	locale := "en-US"
	if req.PreferredLanguage != nil && *req.PreferredLanguage != "" {
		locale = *req.PreferredLanguage
	}

	if emailVal != "" {
		go func() {
			_ = notification.GlobalNotificationService.SendWelcomeEmail(emailVal, req.Name, locale)
		}()
	} else if phoneVal != "" {
		go func() {
			_ = notification.GlobalNotificationService.SendWelcomeSMS(phoneVal, req.Name, locale)
		}()
	}

	if platform := r.Header.Get("X-Platform"); platform != "" {
		identity.Platform = &platform
	}

	resp, err := issueTokens(identity)
	if err != nil {
		s.logger.Error("Failed to issue tokens during registration", zap.Error(err), zap.String("user_id", identity.ID))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// createAccount performs a signup and writes the identity record to the auth
// database. For an active (direct) registration the profile is provisioned in the
// user's chosen data region first (locally for this plane's own region, over the
// internal API for remote regions); a waitlist signup instead parks the provision
// payload on the identity (PendingProvision) and is provisioned at activation.
// Returns a non-zero HTTP status and message on failure.
func (s *Server) createAccount(ctx context.Context, req *api.WaitlistRequest, creds signupCredentials, isActive bool) (*models.Identity, int, apierror.Code, string) {
	var existing models.Identity
	if req.Email != nil && *req.Email != "" {
		if err := database.Auth().Where("LOWER(email) = ?", normalizeEmail(*req.Email)).First(&existing).Error; err == nil {
			return nil, http.StatusBadRequest, apierror.CodeEmailAlreadyExists, "Email already registered"
		}
	}
	if req.PhoneNumber != nil && *req.PhoneNumber != "" {
		if err := database.Auth().Where("phone_number = ?", *req.PhoneNumber).First(&existing).Error; err == nil {
			return nil, http.StatusBadRequest, apierror.CodePhoneAlreadyExists, "Phone number already registered"
		}
	}
	if creds.googleID != "" {
		if err := database.Auth().Where("google_id = ?", creds.googleID).First(&existing).Error; err == nil {
			return nil, http.StatusBadRequest, apierror.CodeAccountAlreadyExists, "Google account already registered"
		}
	}
	if creds.appleID != "" {
		if err := database.Auth().Where("apple_id = ?", creds.appleID).First(&existing).Error; err == nil {
			return nil, http.StatusBadRequest, apierror.CodeAccountAlreadyExists, "Apple account already registered"
		}
	}

	dataRegion, err := residencyRegion(req.DataRegion)
	if err != nil {
		return nil, http.StatusBadRequest, apierror.CodeUnsupportedRegion, "Unsupported data region"
	}

	now := time.Now()
	var termsAcceptedAt, marketingAcceptedAt, aiAcceptedAt *time.Time
	if req.TermsAndConditionsAccepted != nil && *req.TermsAndConditionsAccepted {
		termsAcceptedAt = &now
	}
	if req.MarketingAccepted != nil && *req.MarketingAccepted {
		marketingAcceptedAt = &now
	}
	if req.AiAccepted != nil && *req.AiAccepted {
		aiAcceptedAt = &now
	}

	var emailPtr, phonePtr, googlePtr, applePtr *string
	if req.Email != nil && *req.Email != "" {
		emailPtr = req.Email
	}
	if req.PhoneNumber != nil && *req.PhoneNumber != "" {
		phonePtr = req.PhoneNumber
	}
	if creds.googleID != "" {
		googlePtr = &creds.googleID
	}
	if creds.appleID != "" {
		applePtr = &creds.appleID
	}

	identity := &models.Identity{
		ID:                           uuid.New().String(),
		Email:                        emailPtr,
		PhoneNumber:                  phonePtr,
		Name:                         &req.Name,
		GoogleID:                     googlePtr,
		AppleID:                      applePtr,
		DataRegion:                   &dataRegion,
		IsActive:                     isActive,
		IsEmailVerified:              creds.emailVerified,
		IsPhonenumberVerified:        creds.phoneVerified,
		TermsAndConditionsAcceptedAt: termsAcceptedAt,
		MarketingAcceptedAt:          marketingAcceptedAt,
		AIAcceptedAt:                 aiAcceptedAt,
	}

	provision := regional.UserProvision{
		ID:                           identity.ID,
		Email:                        emailPtr,
		PhoneNumber:                  phonePtr,
		Name:                         &req.Name,
		Gender:                       req.Gender,
		Country:                      req.Country,
		Region:                       req.Region,
		PreferredLanguage:            req.PreferredLanguage,
		DataRegion:                   dataRegion,
		IsActive:                     isActive,
		TermsAndConditionsAcceptedAt: termsAcceptedAt,
		MarketingAcceptedAt:          marketingAcceptedAt,
		AIAcceptedAt:                 aiAcceptedAt,
	}
	if req.DateOfBirth != nil {
		provision.DateOfBirth = &req.DateOfBirth.Time
	}

	if isActive {
		// Direct registration: the user is coming in now, provision the regional
		// profile immediately.
		if err := s.regional.ProvisionInRegion(ctx, dataRegion, provision); err != nil {
			s.logger.Error("failed to provision regional profile", zap.String("data_region", dataRegion), zap.Error(err))
			return nil, http.StatusServiceUnavailable, apierror.CodeInternalError, "Could not create the account in the selected region. Please try again."
		}
	} else {
		// Waitlist signup: defer provisioning to activation. The profile payload is
		// parked on the identity so joining the waitlist never depends on a (possibly
		// remote) data plane being reachable, and no PII lands in a data plane for
		// someone who may never be let in. Activation (SetUserActiveStatus) provisions
		// from this snapshot and clears it.
		pending, err := json.Marshal(provision)
		if err != nil {
			s.logger.Error("failed to serialize pending provision", zap.Error(err), zap.String("user_id", identity.ID))
			return nil, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error"
		}
		pendingStr := string(pending)
		identity.PendingProvision = &pendingStr
	}

	if err := database.Auth().Create(identity).Error; err != nil {
		s.logger.Error("failed to create identity", zap.Error(err), zap.String("user_id", identity.ID))
		return nil, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error"
	}

	return identity, 0, "", ""
}

// ExtractPlatform inspects request headers (X-Platform, Platform, User-Agent) to determine platform.
func ExtractPlatform(r *http.Request) string {
	if r == nil {
		return ""
	}
	if p := r.Header.Get("X-Platform"); p != "" {
		return p
	}
	if p := r.Header.Get("Platform"); p != "" {
		return p
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ios") {
		return "ios"
	}
	if strings.Contains(ua, "android") {
		return "android"
	}
	if strings.Contains(ua, "mozilla") || strings.Contains(ua, "chrome") || strings.Contains(ua, "safari") {
		return "web"
	}
	return ""
}

// UpdateIdentifier implements api.ServerInterface
func (s *Server) UpdateIdentifier(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Unauthorized")
		return
	}

	var req api.UpdateIdentifierJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid payload format")
		return
	}

	reqType := string(req.Type)
	if reqType != "email" && reqType != "phone" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid type")
		return
	}
	if reqType == "email" {
		req.Identifier = normalizeEmail(req.Identifier)
	}

	if !services.GlobalVerificationService.ValidateVerificationID(req.VerificationId, req.Identifier, reqType) {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeVerificationRequired, "Verification required or invalid")
		return
	}

	var identity models.Identity
	if err := database.Auth().Where("id = ?", userID).First(&identity).Error; err != nil {
		s.logger.Error("failed to find identity", zap.String("user_id", userID), zap.Error(err))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	var existing models.Identity
	if reqType == "email" {
		if err := database.Auth().Where("LOWER(email) = ? AND id != ?", req.Identifier, userID).First(&existing).Error; err == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeEmailAlreadyExists, "Email already registered")
			return
		}
	} else {
		if err := database.Auth().Where("phone_number = ? AND id != ?", req.Identifier, userID).First(&existing).Error; err == nil {
			apierror.Write(w, http.StatusBadRequest, apierror.CodePhoneAlreadyExists, "Phone number already registered")
			return
		}
	}

	updates := map[string]interface{}{}
	var updateContact regional.UserContactUpdate
	updateContact.ID = userID

	if reqType == "email" {
		updates["email"] = req.Identifier
		updates["is_email_verified"] = true
		updateContact.Email = &req.Identifier
	} else {
		updates["phone_number"] = req.Identifier
		updates["is_phonenumber_verified"] = true
		updateContact.PhoneNumber = &req.Identifier
	}

	if err := database.Auth().Model(&models.Identity{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		s.logger.Error("failed to update identity", zap.String("user_id", userID), zap.Error(err))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	dataRegion := "eu"
	if identity.DataRegion != nil {
		dataRegion = *identity.DataRegion
	}

	if err := s.regional.UpdateContact(ctx, dataRegion, updateContact); err != nil {
		s.logger.Error("failed to update contact info on data plane", zap.String("user_id", userID), zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{})
}
