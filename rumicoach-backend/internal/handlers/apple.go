package handlers

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"strings"
)

// Sign in with Apple (native iOS flow). The app obtains an identity token from
// ASAuthorizationController and posts it here; we verify Apple's RS256 signature
// against their published JWKS and check the token was minted for one of our app
// bundle IDs (APPLE_CLIENT_IDS). The stable account key is the token's `sub`
// claim (Identity.AppleID): the email may be a per-app @privaterelay.appleid.com
// address and the user's name is only ever provided client-side on the very
// first authorization.
const appleIssuer = "https://appleid.apple.com"

// appleJWKS is a package var so tests can point it at a local key server.
// Apple's signing keys are RSA, so the ECDSA cache in pkg/auth is not reusable.
var appleJWKS = newAppleKeySet("https://appleid.apple.com/auth/keys")

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// appleKeySet fetches and caches Apple's public signing keys. Same shape and
// refresh policy as the internal JWKS cache in pkg/auth/jwks.go: periodic
// refresh, a minimum gap so garbage tokens with unknown kids can't trigger a
// fetch stampede, and stale keys served if a refresh fails.
type appleKeySet struct {
	url    string
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

const (
	appleJWKSRefreshInterval = 15 * time.Minute
	appleJWKSMinRefreshGap   = 30 * time.Second
)

func newAppleKeySet(url string) *appleKeySet {
	return &appleKeySet{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		keys:   map[string]*rsa.PublicKey{},
	}
}

func (c *appleKeySet) get(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	stale := time.Since(c.fetchedAt) > appleJWKSRefreshInterval
	c.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}
	if err := c.fetch(); err != nil {
		if ok {
			return key, nil // serve stale key rather than failing verification
		}
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("unknown Apple key id %q", kid)
}

func (c *appleKeySet) fetch() error {
	c.mu.Lock()
	if time.Since(c.fetchedAt) < appleJWKSMinRefreshGap {
		c.mu.Unlock()
		return nil
	}
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	resp, err := c.client.Get(c.url)
	if err != nil {
		return fmt.Errorf("fetch Apple JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch Apple JWKS: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var set struct {
		Keys []appleJWK `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("parse Apple JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("Apple JWKS at %s contains no usable keys", c.url)
	}

	c.mu.Lock()
	c.keys = keys
	c.mu.Unlock()
	return nil
}

// validateAppleToken verifies a Sign in with Apple identity token and returns its
// stable subject (`sub`) together with the email claim and the flags that decide
// what that email may be used for. Only what the token itself carries is returned —
// a client-supplied email alongside it proves nothing.
func (s *Server) validateAppleToken(identityToken string) (ssoToken, int, error) {
	configIDs := strings.Split(config.AppConfig.AppleClientIDs, ",")

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(identityToken, claims, func(token *jwt.Token) (interface{}, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("token has no key id")
		}
		return appleJWKS.get(kid)
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(appleIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		// A rejected token is the normal outcome of an expired or forged credential.
		s.logger.Info("Apple identity token validation failed", zap.Error(err))
		return ssoToken{}, http.StatusUnauthorized, fmt.Errorf("Invalid Apple Token")
	}

	audiences, _ := claims.GetAudience()
	audOK := false
	for _, aud := range audiences {
		for _, clientID := range configIDs {
			if clientID = strings.TrimSpace(clientID); clientID != "" && clientID == aud {
				audOK = true
			}
		}
	}
	if !audOK {
		s.logger.Info("Apple identity token audience not accepted", zap.Strings("aud", audiences))
		return ssoToken{}, http.StatusUnauthorized, fmt.Errorf("Invalid Apple Token")
	}

	sub, _ := claims.GetSubject()
	if sub == "" {
		return ssoToken{}, http.StatusUnauthorized, fmt.Errorf("Incomplete user info from Apple")
	}

	email, _ := claims["email"].(string)
	// Either signal is enough to recognize a "Hide My Email" address: the claim is
	// authoritative, the domain is the visible fallback if it ever stops being sent.
	privateRelay := strings.HasSuffix(strings.ToLower(email), "@privaterelay.appleid.com")
	if v := claimBool(claims["is_private_email"]); v != nil && *v {
		privateRelay = true
	}

	tok := ssoToken{
		subject:       sub,
		email:         email,
		emailVerified: claimBool(claims["email_verified"]),
		privateRelay:  privateRelay,
	}
	if !tok.emailIsVerified() {
		// Apple sends email_verified on every token that carries an email, so this is
		// worth seeing in the logs: it is either an account Apple itself distrusts or a
		// payload we no longer understand.
		s.logger.Warn("Apple identity token reports an unverified email; it will not be used to match an account")
	}
	return tok, http.StatusOK, nil
}

// AppleLogin implements api.ServerInterface. Login only — mirrors GoogleLogin:
// it links by apple_id, falls back to the verified token email for accounts
// created before Apple sign-in existed, and never creates accounts.
func (s *Server) AppleLogin(w http.ResponseWriter, r *http.Request) {
	var req api.AppleLoginJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// The body carries a live credential; never log it (see GoogleLogin).
		s.logger.Info("could not decode AppleLogin request", zap.Error(err))
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}
	if req.IdentityToken == "" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "identityToken is required")
		return
	}

	tok, status, err := s.validateAppleToken(req.IdentityToken)
	if err != nil {
		apierror.Write(w, status, apierror.CodeAppleTokenInvalid, err.Error())
		return
	}

	var identity models.Identity
	if err := database.Auth().Where("apple_id = ?", tok.subject).First(&identity).Error; err != nil {
		// The req.Email/req.Name fields the app sends are deliberately ignored here:
		// linking by an unverified, client-chosen email would let any Apple user
		// attach their credential to an arbitrary account.
		found := false
		if linkEmail := tok.linkableEmail(); linkEmail != "" {
			if errEmail := database.Auth().Where("LOWER(email) = ?", normalizeEmail(linkEmail)).First(&identity).Error; errEmail == nil {
				if identity.AppleID != nil && *identity.AppleID != "" && *identity.AppleID != tok.subject {
					// The account answers to another Apple ID whose email has since moved
					// to this one. Not a privilege escalation — Apple only puts a verified
					// address in the token, and whoever holds that mailbox can already log
					// in by OTP — but rare enough to be worth seeing in the logs.
					s.logger.Warn("replacing the Apple ID on an account matched by verified email",
						zap.String("user_id", identity.ID))
				}
				if err := linkProviderID(identity.ID, "apple_id", tok.subject); err != nil {
					s.logger.Error("failed to link Apple ID", zap.Error(err), zap.String("user_id", identity.ID))
					apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
					return
				}
				identity.AppleID = &tok.subject
				found = true
			}
		}
		if !found {
			// Two dead ends, told apart because the remedies differ. A private-relay
			// token can NEVER be matched by email — the address is per-app, so an
			// account created with any other provider cannot carry it — and the only
			// way in is LinkAppleAccount from an authenticated session. Everything
			// else is genuinely an unknown account.
			if tok.privateRelay {
				apierror.Write(w, http.StatusForbidden, apierror.CodeApplePrivateEmailUnlinked,
					"This Apple ID is not linked to an account yet. Sign in the way you signed up, then link Apple from your account.")
				return
			}
			if tok.email != "" && !tok.emailIsVerified() {
				apierror.Write(w, http.StatusForbidden, apierror.CodeSSOEmailNotVerified,
					"Apple reports this email as unverified, so it cannot be matched to an account.")
				return
			}
			apierror.Write(w, http.StatusForbidden, apierror.CodeAccountNotFound, "No account found. Please sign up first.")
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

// LinkAppleAccount implements api.ServerInterface. Attaches a Sign in with Apple
// credential to the CURRENT account, so the user can sign in with Apple from then
// on regardless of which provider created the account.
//
// This is the only way in for "Hide My Email" users: their token carries a per-app
// @privaterelay.appleid.com address that can never equal the email of an account
// created with Google or by OTP, so AppleLogin's email fallback has nothing to
// match on and no ordering of the lookups can change that. Here the account is
// already proven by the bearer token, so no email match is needed — and none is
// attempted: requiring the Apple email to equal the account email would lock out
// exactly the users this endpoint exists for.
func (s *Server) LinkAppleAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Unauthorized")
		return
	}

	var req api.LinkAppleAccountJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// The body carries a live credential; never log it (see GoogleLogin).
		s.logger.Info("could not decode LinkAppleAccount request", zap.Error(err))
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request format")
		return
	}
	if req.IdentityToken == "" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "identityToken is required")
		return
	}

	tok, status, err := s.validateAppleToken(req.IdentityToken)
	if err != nil {
		apierror.Write(w, status, apierror.CodeAppleTokenInvalid, err.Error())
		return
	}

	var other models.Identity
	if err := database.Auth().Where("apple_id = ? AND id != ?", tok.subject, userID).First(&other).Error; err == nil {
		// Someone else already signs in with this Apple ID. Moving it would lock them
		// out of their own account.
		apierror.Write(w, http.StatusBadRequest, apierror.CodeAccountAlreadyExists,
			"This Apple ID is already linked to another account.")
		return
	}

	var identity models.Identity
	if err := database.Auth().Where("id = ?", userID).First(&identity).Error; err != nil {
		s.logger.Error("failed to find identity", zap.String("user_id", userID), zap.Error(err))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}
	if identity.AppleID != nil && *identity.AppleID == tok.subject {
		// Already linked — the app can call this after a failed login without having
		// to know whether a previous attempt got through.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
		return
	}
	if identity.AppleID != nil && *identity.AppleID != "" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeAppleAlreadyLinked,
			"This account is already linked to a different Apple ID.")
		return
	}

	if err := linkProviderID(userID, "apple_id", tok.subject); err != nil {
		s.logger.Error("failed to link Apple ID", zap.Error(err), zap.String("user_id", userID))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{})
}
