package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rumi/rumi-be/config"
)

// TokenSigner issues ES256-signed access tokens and exposes the public keys
// (as a JWKS document) that data planes use to verify them.
type TokenSigner interface {
	// SignToken signs the given claims and returns the compact JWT.
	SignToken(claims jwt.Claims) (string, error)
	// JWKS returns the JSON-encoded JWK Set of all currently valid public keys.
	JWKS(ctx context.Context) ([]byte, error)
}

var globalSigner TokenSigner

// InitSigner initializes the token signer for this deployment. On the auth plane
// (AUTH_KMS_KEY_ID set) it signs via Cloud KMS; in development it falls back to an
// ephemeral in-process P-256 key. Data planes get no signer and cannot issue tokens.
func InitSigner(ctx context.Context) error {
	cfg := config.AppConfig
	if cfg.AuthKMSKeyID != "" {
		s, err := newKMSSigner(ctx, cfg.AuthKMSKeyID)
		if err != nil {
			return fmt.Errorf("init KMS signer: %w", err)
		}
		globalSigner = s
		return nil
	}
	if cfg.Environment == "development" {
		s, err := newDevSigner()
		if err != nil {
			return fmt.Errorf("init dev signer: %w", err)
		}
		globalSigner = s
		return nil
	}
	return nil
}

// Signer returns the active token signer, or nil on data planes that cannot sign.
func Signer() TokenSigner {
	return globalSigner
}

// JWKSHandler serves this deployment's public signing keys at
// /.well-known/jwks.json. Only registered on the auth plane.
func JWKSHandler(w http.ResponseWriter, r *http.Request) {
	signer := Signer()
	if signer == nil {
		http.Error(w, `{"error": "not an auth service"}`, http.StatusNotFound)
		return
	}
	jwks, err := signer.JWKS(r.Context())
	if err != nil {
		http.Error(w, `{"error": "keys unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(jwks)
}
