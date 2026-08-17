package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/iterator"
)

// kmsSigner signs tokens with a Cloud KMS EC_SIGN_P256_SHA256 asymmetric key.
// The private key never leaves KMS; rotation is handled by creating a new key
// version — all enabled versions are published in the JWKS so old tokens keep
// verifying until they expire.
type kmsSigner struct {
	client  *kms.KeyManagementClient
	keyName string // projects/.../cryptoKeys/<key>

	mu          sync.Mutex
	signVersion string // newest enabled version, used for signing
	signKID     string
	pubKeys     map[string]*ecdsa.PublicKey // kid → public key, for same-process verification
	jwks        []byte
	refreshedAt time.Time
}

const kmsRefreshInterval = 15 * time.Minute

func newKMSSigner(ctx context.Context, keyName string) (*kmsSigner, error) {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, err
	}
	s := &kmsSigner{client: client, keyName: keyName}
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// refresh lists enabled key versions, picks the newest for signing, and rebuilds the JWKS.
func (s *kmsSigner) refresh(ctx context.Context) error {
	it := s.client.ListCryptoKeyVersions(ctx, &kmspb.ListCryptoKeyVersionsRequest{
		Parent: s.keyName,
	})

	type keyVersion struct {
		name string
		kid  string
		pub  *ecdsa.PublicKey
	}
	var versions []keyVersion

	for {
		v, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("list key versions: %w", err)
		}
		if v.State != kmspb.CryptoKeyVersion_ENABLED {
			continue
		}
		pub, err := s.fetchPublicKey(ctx, v.Name)
		if err != nil {
			return fmt.Errorf("fetch public key %s: %w", v.Name, err)
		}
		versions = append(versions, keyVersion{name: v.Name, kid: kmsKID(v.Name), pub: pub})
	}

	if len(versions) == 0 {
		return fmt.Errorf("no enabled versions for KMS key %s", s.keyName)
	}

	// Versions are named .../cryptoKeyVersions/<n>; the last listed is the newest.
	newest := versions[len(versions)-1]

	keys := make([]jwk, 0, len(versions))
	pubKeys := make(map[string]*ecdsa.PublicKey, len(versions))
	for _, v := range versions {
		keys = append(keys, jwkFromECDSA(v.pub, v.kid))
		pubKeys[v.kid] = v.pub
	}
	jwksBytes, err := json.Marshal(jwkSet{Keys: keys})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.signVersion = newest.name
	s.signKID = newest.kid
	s.pubKeys = pubKeys
	s.jwks = jwksBytes
	s.refreshedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// publicKeyFor returns the public key for a kid this signer manages, allowing the
// auth plane to verify its own tokens without fetching its JWKS over HTTP.
func (s *kmsSigner) publicKeyFor(kid string) (*ecdsa.PublicKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pub, ok := s.pubKeys[kid]
	return pub, ok
}

func (s *kmsSigner) fetchPublicKey(ctx context.Context, versionName string) (*ecdsa.PublicKey, error) {
	resp, err := s.client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: versionName})
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(resp.Pem))
	if block == nil {
		return nil, fmt.Errorf("invalid PEM from KMS")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("KMS key is not ECDSA (got %T)", pub)
	}
	return ecPub, nil
}

func (s *kmsSigner) maybeRefresh(ctx context.Context) {
	s.mu.Lock()
	stale := time.Since(s.refreshedAt) > kmsRefreshInterval
	s.mu.Unlock()
	if stale {
		_ = s.refresh(ctx) // best effort; keep serving previous state on failure
	}
}

func (s *kmsSigner) SignToken(claims jwt.Claims) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.maybeRefresh(ctx)

	s.mu.Lock()
	version := s.signVersion
	kid := s.signKID
	s.mu.Unlock()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid

	signingString, err := token.SigningString()
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256([]byte(signingString))
	resp, err := s.client.AsymmetricSign(ctx, &kmspb.AsymmetricSignRequest{
		Name: version,
		Digest: &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{Sha256: digest[:]},
		},
	})
	if err != nil {
		return "", fmt.Errorf("KMS sign: %w", err)
	}

	sig, err := derToRawES256(resp.Signature)
	if err != nil {
		return "", err
	}
	return signingString + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *kmsSigner) JWKS(ctx context.Context) ([]byte, error) {
	s.maybeRefresh(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jwks) == 0 {
		return nil, fmt.Errorf("JWKS not available")
	}
	return s.jwks, nil
}

// kmsKID derives a stable key ID from a KMS version resource name
// (projects/.../cryptoKeyVersions/<n> → "kms-<n>").
func kmsKID(versionName string) string {
	parts := strings.Split(versionName, "/")
	return "kms-" + parts[len(parts)-1]
}

// derToRawES256 converts a DER-encoded ECDSA signature (as returned by KMS)
// to the raw fixed-width R||S form required by JWS ES256.
func derToRawES256(der []byte) ([]byte, error) {
	var parsed struct {
		R, S *big.Int
	}
	if _, err := asn1.Unmarshal(der, &parsed); err != nil {
		return nil, fmt.Errorf("parse DER signature: %w", err)
	}
	out := make([]byte, 64)
	parsed.R.FillBytes(out[:32])
	parsed.S.FillBytes(out[32:])
	return out, nil
}
