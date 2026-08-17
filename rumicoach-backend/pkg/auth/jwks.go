package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwk is the subset of RFC 7517 needed for P-256 signing keys.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

func jwkFromECDSA(pub *ecdsa.PublicKey, kid string) jwk {
	byteLen := (pub.Curve.Params().BitSize + 7) / 8
	x := make([]byte, byteLen)
	y := make([]byte, byteLen)
	pub.X.FillBytes(x)
	pub.Y.FillBytes(y)
	return jwk{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(x),
		Y:   base64.RawURLEncoding.EncodeToString(y),
		Kid: kid,
		Use: "sig",
		Alg: "ES256",
	}
}

func (k jwk) toECDSA() (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" || k.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported JWK type %s/%s", k.Kty, k.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

// jwksCache fetches and caches the auth service's public keys for offline token
// verification on data planes.
type jwksCache struct {
	url    string
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

const (
	jwksRefreshInterval = 15 * time.Minute
	// jwksMinRefreshGap limits how often an unknown kid can trigger a refetch,
	// so unverified garbage tokens can't be used to hammer the auth service.
	jwksMinRefreshGap = 30 * time.Second
)

var globalJWKS *jwksCache

// InitJWKSCache configures JWKS-based verification against the given URL.
func InitJWKSCache(url string) {
	globalJWKS = &jwksCache{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		keys:   map[string]*ecdsa.PublicKey{},
	}
}

func (c *jwksCache) get(kid string) (*ecdsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	stale := time.Since(c.fetchedAt) > jwksRefreshInterval
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
	return nil, fmt.Errorf("unknown key id %q", kid)
}

func (c *jwksCache) fetch() error {
	c.mu.Lock()
	if time.Since(c.fetchedAt) < jwksMinRefreshGap {
		c.mu.Unlock()
		return nil
	}
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	resp, err := c.client.Get(c.url)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	keys := make(map[string]*ecdsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		pub, err := k.toECDSA()
		if err != nil {
			continue // skip unsupported key types
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return fmt.Errorf("JWKS at %s contains no usable keys", c.url)
	}

	c.mu.Lock()
	c.keys = keys
	c.mu.Unlock()
	return nil
}
