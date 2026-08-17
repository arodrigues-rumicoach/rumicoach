package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"

	"github.com/golang-jwt/jwt/v5"
)

// devSigner signs tokens with an ephemeral in-process P-256 key. Development only:
// tokens do not survive restarts and the JWKS is served from the same process.
type devSigner struct {
	key  *ecdsa.PrivateKey
	kid  string
	jwks []byte
}

func newDevSigner() (*devSigner, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	const kid = "dev-1"
	jwksBytes, err := json.Marshal(jwkSet{Keys: []jwk{jwkFromECDSA(&key.PublicKey, kid)}})
	if err != nil {
		return nil, err
	}
	return &devSigner{key: key, kid: kid, jwks: jwksBytes}, nil
}

func (s *devSigner) SignToken(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = s.kid
	return token.SignedString(s.key)
}

func (s *devSigner) JWKS(ctx context.Context) ([]byte, error) {
	return s.jwks, nil
}

// publicKey exposes the dev public key so the same process can verify its own
// tokens without an HTTP JWKS fetch.
func (s *devSigner) publicKey() *ecdsa.PublicKey {
	return &s.key.PublicKey
}
