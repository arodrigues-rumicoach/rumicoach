package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const appleTestKid = "apple-test-key"
const appleTestAudience = "coach.rumi.app.qa"

// setupAppleTest wires an in-memory identity DB, a fake Apple JWKS endpoint, and a
// config that accepts appleTestAudience. Returns the server and the JWKS signing key.
func setupAppleTest(t *testing.T) (*Server, *rsa.PrivateKey) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created tables (see balance_test.go): SQLite + the models' Postgres
	// column types don't AutoMigrate cleanly.
	for _, ddl := range []string{
		`CREATE TABLE identities (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE,
			phone_number TEXT,
			name TEXT,
			google_id TEXT UNIQUE,
			apple_id TEXT UNIQUE,
			data_region TEXT,
			platform TEXT,
			last_online_at DATETIME,
			is_active BOOLEAN DEFAULT FALSE,
			is_admin BOOLEAN DEFAULT FALSE,
			is_email_verified BOOLEAN DEFAULT FALSE,
			is_phonenumber_verified BOOLEAN DEFAULT FALSE,
			refresh_token_hash TEXT,
			refresh_token_expires_at DATETIME,
			terms_and_conditions_accepted_at DATETIME,
			marketing_accepted_at DATETIME,
			ai_accepted_at DATETIME,
			pending_provision TEXT,
			created_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			last_online_at DATETIME,
			last_platform TEXT
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}
	prevDB, prevAuthDB := database.DB, database.AuthDB
	database.DB, database.AuthDB = db, db
	t.Cleanup(func() { database.DB, database.AuthDB = prevDB, prevAuthDB })

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": appleTestKid,
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	prevJWKS := appleJWKS
	appleJWKS = newAppleKeySet(jwks.URL)
	t.Cleanup(func() { appleJWKS = prevJWKS })

	prevConfig := config.AppConfig
	config.AppConfig = &config.Config{
		Environment:       "development",
		AppleClientIDs:    "coach.rumi.app," + appleTestAudience,
		JWTSecret:         "test-secret",
		AllowLegacyHS256:  true,
		AccessTokenExpire: 30,
	}
	t.Cleanup(func() { config.AppConfig = prevConfig })

	return NewServer(zap.NewNop(), nil, nil, nil), key
}

// mintAppleToken signs an identity token like Apple would. Overrides merge into a
// valid default claim set.
func mintAppleToken(t *testing.T, key *rsa.PrivateKey, overrides map[string]any) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": appleIssuer,
		"aud": appleTestAudience,
		"sub": "001234.abcdef.5678",
		// Apple sends these two as JSON strings, not bools — the defaults mirror a real
		// "Hide My Email" token.
		"email":            "user@privaterelay.appleid.com",
		"email_verified":   "true",
		"is_private_email": "true",
		"exp":              time.Now().Add(10 * time.Minute).Unix(),
		"iat":              time.Now().Unix(),
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = appleTestKid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func postAppleLogin(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/auth/apple", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	server.AppleLogin(w, req)
	return w
}

func TestValidateAppleToken(t *testing.T) {
	server, key := setupAppleTest(t)

	t.Run("valid token", func(t *testing.T) {
		tok, _, err := server.validateAppleToken(mintAppleToken(t, key, nil))
		if err != nil {
			t.Fatalf("expected valid token, got %v", err)
		}
		if tok.subject != "001234.abcdef.5678" || tok.email != "user@privaterelay.appleid.com" {
			t.Fatalf("unexpected claims: %q %q", tok.subject, tok.email)
		}
		if !tok.emailIsVerified() || !tok.privateRelay {
			t.Fatalf("expected a verified private-relay address, got %+v", tok)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		_, status, err := server.validateAppleToken(mintAppleToken(t, key, map[string]any{"aud": "com.evil.app"}))
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		_, status, err := server.validateAppleToken(mintAppleToken(t, key, map[string]any{"iss": "https://accounts.google.com"}))
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		_, status, err := server.validateAppleToken(mintAppleToken(t, key, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}))
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})

	t.Run("missing expiry", func(t *testing.T) {
		_, status, err := server.validateAppleToken(mintAppleToken(t, key, map[string]any{"exp": nil}))
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})

	t.Run("HS256 forgery is rejected", func(t *testing.T) {
		claims := jwt.MapClaims{"iss": appleIssuer, "aud": appleTestAudience, "sub": "x", "exp": time.Now().Add(time.Hour).Unix()}
		forged := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		forged.Header["kid"] = appleTestKid
		signed, _ := forged.SignedString([]byte("guessable"))
		_, status, err := server.validateAppleToken(signed)
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})

	t.Run("garbage", func(t *testing.T) {
		_, status, err := server.validateAppleToken("not-a-jwt")
		if err == nil || status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d %v", status, err)
		}
	})
}

func TestAppleLoginExistingAppleID(t *testing.T) {
	server, key := setupAppleTest(t)
	name := "Filipa"
	appleID := "001234.abcdef.5678"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, AppleID: &appleID, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, mintAppleToken(t, key, nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp api.TokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.AccessToken == nil || *resp.AccessToken == "" {
		t.Fatalf("expected tokens, got %s (err %v)", w.Body.String(), err)
	}
}

func TestAppleLoginLinksByVerifiedEmail(t *testing.T) {
	server, key := setupAppleTest(t)
	name := "Filipa"
	email := "filipa@example.com"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{"email": "filipa@example.com"})
	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var identity models.Identity
	if err := database.Auth().Where("id = ?", "user-1").First(&identity).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if identity.AppleID == nil || *identity.AppleID != "001234.abcdef.5678" {
		t.Fatalf("expected apple_id linked, got %v", identity.AppleID)
	}
}

func TestAppleLoginUnknownAccount(t *testing.T) {
	server, key := setupAppleTest(t)
	token := mintAppleToken(t, key, map[string]any{"email": "nobody@example.com", "is_private_email": nil})
	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("ACCOUNT_NOT_FOUND")) {
		t.Fatalf("expected ACCOUNT_NOT_FOUND, got %s", w.Body.String())
	}
}

func TestAppleLoginPrivateRelayIsItsOwnDeadEnd(t *testing.T) {
	// "Hide My Email" against an account created with Google: the relay address is
	// per-app, so no email lookup can ever reach that account. The distinct code is
	// what lets the app say "link Apple from your account" instead of the useless
	// "no account found — sign up first".
	server, key := setupAppleTest(t)
	name, email := "Filipa", "filipa@gmail.com"
	googleID := "google-sub-1"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, GoogleID: &googleID, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, mintAppleToken(t, key, nil)))
	if w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("APPLE_PRIVATE_EMAIL_UNLINKED")) {
		t.Fatalf("expected 403 APPLE_PRIVATE_EMAIL_UNLINKED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppleLoginUnverifiedEmailCannotLink(t *testing.T) {
	// The takeover this gate exists for: a token whose email the provider itself does
	// not vouch for must not find an account, however well the address matches.
	server, key := setupAppleTest(t)
	name, email := "Victim", "victim@example.com"
	if err := database.Auth().Create(&models.Identity{
		ID: "victim-1", Name: &name, Email: &email, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{
		"email": "victim@example.com", "email_verified": "false", "is_private_email": nil,
	})
	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token))
	if w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("SSO_EMAIL_NOT_VERIFIED")) {
		t.Fatalf("expected 403 SSO_EMAIL_NOT_VERIFIED, got %d: %s", w.Code, w.Body.String())
	}
	var identity models.Identity
	database.Auth().Where("id = ?", "victim-1").First(&identity)
	if identity.AppleID != nil {
		t.Fatalf("victim account must not gain an apple_id")
	}
}

// A token that carries no email_verified claim at all still links: both providers
// always send it, so its absence means the payload changed shape — and locking
// every user out is a worse failure than the attack the gate blocks, which needs
// an explicit false.
func TestAppleLoginMissingVerifiedClaimStillLinks(t *testing.T) {
	server, key := setupAppleTest(t)
	name, email := "Filipa", "filipa@example.com"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{
		"email": "filipa@example.com", "email_verified": nil, "is_private_email": nil,
	})
	if w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token)); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func postLinkApple(t *testing.T, server *Server, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/auth/me/link/apple", bytes.NewBufferString(body))
	if userID != "" {
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
	}
	w := httptest.NewRecorder()
	server.LinkAppleAccount(w, req)
	return w
}

func TestLinkAppleAccount(t *testing.T) {
	server, key := setupAppleTest(t)
	name, email := "Filipa", "filipa@gmail.com"
	googleID := "google-sub-1"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, GoogleID: &googleID, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	token := mintAppleToken(t, key, nil) // "Hide My Email": nothing matches by email

	t.Run("unauthenticated rejected", func(t *testing.T) {
		w := postLinkApple(t, server, "", fmt.Sprintf(`{"identityToken": %q}`, token))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		w := postLinkApple(t, server, "user-1", `{"identityToken": "garbage"}`)
		if w.Code != http.StatusUnauthorized || !bytes.Contains(w.Body.Bytes(), []byte("APPLE_TOKEN_INVALID")) {
			t.Fatalf("expected 401 APPLE_TOKEN_INVALID, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("links a private-relay Apple ID to a Google account", func(t *testing.T) {
		w := postLinkApple(t, server, "user-1", fmt.Sprintf(`{"identityToken": %q}`, token))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var identity models.Identity
		database.Auth().Where("id = ?", "user-1").First(&identity)
		if identity.AppleID == nil || *identity.AppleID != "001234.abcdef.5678" {
			t.Fatalf("expected apple_id linked, got %v", identity.AppleID)
		}
		// And the login that failed before now works.
		if lw := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token)); lw.Code != http.StatusOK {
			t.Fatalf("expected Apple login to work after linking, got %d: %s", lw.Code, lw.Body.String())
		}
	})

	t.Run("relinking the same Apple ID is a no-op", func(t *testing.T) {
		if w := postLinkApple(t, server, "user-1", fmt.Sprintf(`{"identityToken": %q}`, token)); w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("an Apple ID owned by another account is refused", func(t *testing.T) {
		other := "Other"
		if err := database.Auth().Create(&models.Identity{ID: "user-2", Name: &other, IsActive: true}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		w := postLinkApple(t, server, "user-2", fmt.Sprintf(`{"identityToken": %q}`, token))
		if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("ACCOUNT_ALREADY_EXISTS")) {
			t.Fatalf("expected 400 ACCOUNT_ALREADY_EXISTS, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("an account already linked to a different Apple ID is refused", func(t *testing.T) {
		second := mintAppleToken(t, key, map[string]any{"sub": "002222.zzzz.9999"})
		w := postLinkApple(t, server, "user-1", fmt.Sprintf(`{"identityToken": %q}`, second))
		if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("APPLE_ALREADY_LINKED")) {
			t.Fatalf("expected 400 APPLE_ALREADY_LINKED, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestAppleLoginClientEmailCannotLink(t *testing.T) {
	// A client-supplied email must never link accounts: only the email inside the
	// verified token may. Otherwise a valid token for attacker@ could take over any
	// account by posting {"email": "victim@example.com"}.
	server, key := setupAppleTest(t)
	name := "Victim"
	email := "victim@example.com"
	if err := database.Auth().Create(&models.Identity{
		ID: "victim-1", Name: &name, Email: &email, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{"email": nil}) // token carries no email
	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q, "email": "victim@example.com"}`, token))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var identity models.Identity
	database.Auth().Where("id = ?", "victim-1").First(&identity)
	if identity.AppleID != nil {
		t.Fatalf("victim account must not gain an apple_id")
	}
}

func TestAppleLoginInactiveAccount(t *testing.T) {
	server, key := setupAppleTest(t)
	name := "Waiting"
	appleID := "001234.abcdef.5678"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, AppleID: &appleID, IsActive: false,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, mintAppleToken(t, key, nil)))
	if w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("ACCOUNT_NOT_ACTIVE")) {
		t.Fatalf("expected 403 ACCOUNT_NOT_ACTIVE, got %d: %s", w.Code, w.Body.String())
	}
}

func postVerifySSO(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/auth/verifications/sso", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	server.VerifySSO(w, req)
	return w
}

func TestVerifySSOApple(t *testing.T) {
	server, key := setupAppleTest(t)

	t.Run("new account passes", func(t *testing.T) {
		w := postVerifySSO(t, server, fmt.Sprintf(`{"type": "apple", "idToken": %q}`, mintAppleToken(t, key, nil)))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("existing apple account rejected", func(t *testing.T) {
		name := "Filipa"
		appleID := "001234.abcdef.5678"
		if err := database.Auth().Create(&models.Identity{
			ID: "user-1", Name: &name, AppleID: &appleID, IsActive: true,
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		w := postVerifySSO(t, server, fmt.Sprintf(`{"type": "apple", "idToken": %q}`, mintAppleToken(t, key, nil)))
		if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("ACCOUNT_ALREADY_EXISTS")) {
			t.Fatalf("expected 400 ACCOUNT_ALREADY_EXISTS, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		w := postVerifySSO(t, server, `{"type": "apple", "idToken": "garbage"}`)
		if w.Code != http.StatusUnauthorized || !bytes.Contains(w.Body.Bytes(), []byte("APPLE_TOKEN_INVALID")) {
			t.Fatalf("expected 401 APPLE_TOKEN_INVALID, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestSignupRejectsCaseVariantEmailDuplicate(t *testing.T) {
	// QA-found: a Google account stored as "Mando@x.com" and an Apple signup whose
	// token says "mando@x.com" produced two accounts for one mailbox, because every
	// existence check compared emails case-sensitively.
	server, key := setupAppleTest(t)
	name := "Filipa"
	googleID := "google-sub-1"
	email := "Filipa@Example.com" // legacy row stored with client casing
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, GoogleID: &googleID, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{"email": "filipa@example.com"})

	t.Run("VerifySSO pre-check rejects", func(t *testing.T) {
		w := postVerifySSO(t, server, fmt.Sprintf(`{"type": "apple", "idToken": %q}`, token))
		if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("EMAIL_ALREADY_EXISTS")) {
			t.Fatalf("expected 400 EMAIL_ALREADY_EXISTS, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("createAccount rejects", func(t *testing.T) {
		req := &api.WaitlistRequest{Name: "Filipa", AppleIdentityToken: &token}
		creds, status, _, _ := server.verifySignupCredentials(t.Context(), req)
		if status != 0 {
			t.Fatalf("credentials should verify, got %d", status)
		}
		_, status, code, _ := server.createAccount(t.Context(), req, creds, true)
		if status != http.StatusBadRequest || code != "EMAIL_ALREADY_EXISTS" {
			t.Fatalf("expected 400 EMAIL_ALREADY_EXISTS, got %d %s", status, code)
		}
	})
}

func TestAppleLoginLinksEmailCaseInsensitively(t *testing.T) {
	server, key := setupAppleTest(t)
	name := "Filipa"
	email := "Filipa@Example.com" // legacy casing
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, Email: &email, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	token := mintAppleToken(t, key, map[string]any{"email": "filipa@example.com"})
	w := postAppleLogin(t, server, fmt.Sprintf(`{"identityToken": %q}`, token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var identity models.Identity
	if err := database.Auth().Where("id = ?", "user-1").First(&identity).Error; err != nil || identity.AppleID == nil {
		t.Fatalf("expected apple_id linked to the legacy-cased account (err %v)", err)
	}
}

func TestEraseIdentityClearsAppleID(t *testing.T) {
	server, _ := setupAppleTest(t)
	_ = server
	name := "Filipa"
	appleID := "001234.abcdef.5678"
	googleID := "google-sub-1"
	if err := database.Auth().Create(&models.Identity{
		ID: "user-1", Name: &name, AppleID: &appleID, GoogleID: &googleID, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := eraseIdentity("user-1"); err != nil {
		t.Fatalf("eraseIdentity: %v", err)
	}
	var identity models.Identity
	if err := database.Auth().Where("id = ?", "user-1").First(&identity).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	// A lingering apple_id would keep the deleted account squatting on the unique
	// index: the user could never sign up with that Apple ID again, and Apple login
	// would resolve to the anonymized, inactive account.
	if identity.AppleID != nil || identity.GoogleID != nil {
		t.Fatalf("expected provider IDs cleared, got apple=%v google=%v", identity.AppleID, identity.GoogleID)
	}
}

func TestVerifySignupCredentialsApple(t *testing.T) {
	server, key := setupAppleTest(t)

	t.Run("token email becomes authoritative", func(t *testing.T) {
		token := mintAppleToken(t, key, nil)
		req := &api.WaitlistRequest{Name: "Filipa", AppleIdentityToken: &token}
		creds, status, code, msg := server.verifySignupCredentials(t.Context(), req)
		if status != 0 {
			t.Fatalf("expected success, got %d %s %s", status, code, msg)
		}
		if creds.appleID != "001234.abcdef.5678" || !creds.emailVerified {
			t.Fatalf("unexpected creds: %+v", creds)
		}
		if req.Email == nil || *req.Email != "user@privaterelay.appleid.com" {
			t.Fatalf("expected req.Email filled from token, got %v", req.Email)
		}
	})

	t.Run("email mismatch rejected", func(t *testing.T) {
		token := mintAppleToken(t, key, nil)
		other := "other@example.com"
		req := &api.WaitlistRequest{Name: "Filipa", AppleIdentityToken: &token, Email: &other}
		_, status, code, _ := server.verifySignupCredentials(t.Context(), req)
		if status != http.StatusBadRequest || code != "APPLE_EMAIL_MISMATCH" {
			t.Fatalf("expected APPLE_EMAIL_MISMATCH, got %d %s", status, code)
		}
	})

	t.Run("token without email rejected", func(t *testing.T) {
		token := mintAppleToken(t, key, map[string]any{"email": nil})
		req := &api.WaitlistRequest{Name: "Filipa", AppleIdentityToken: &token}
		_, status, code, _ := server.verifySignupCredentials(t.Context(), req)
		if status != http.StatusBadRequest || code != "APPLE_EMAIL_MISSING" {
			t.Fatalf("expected APPLE_EMAIL_MISSING, got %d %s", status, code)
		}
	})
}
