package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rumi/rumi-be/config"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	Email                 string `json:"email"`
	Name                  string `json:"name"`
	IsActive              bool   `json:"is_active"`
	IsEmailVerified       bool   `json:"is_email_verified"`
	IsPhonenumberVerified bool   `json:"is_phonenumber_verified"`
	IsAdmin               bool   `json:"is_admin"`
	// Region is the user's data-residency region ("eu" / "us"). Data planes reject
	// tokens whose region does not match their own, so a token can never read or
	// write data outside the user's chosen region.
	Region string `json:"rgn,omitempty"`
	jwt.RegisteredClaims
}

func GetPasswordHash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func CreateAccessToken(userID, email, name string, isActive, isEmailVerified, isPhoneVerified, isAdmin bool, region string) (string, error) {
	expirationTime := time.Now().Add(time.Duration(config.AppConfig.AccessTokenExpire) * time.Minute)

	claims := &Claims{
		Email:                 email,
		Name:                  name,
		IsActive:              isActive,
		IsEmailVerified:       isEmailVerified,
		IsPhonenumberVerified: isPhoneVerified,
		IsAdmin:               isAdmin,
		Region:                region,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	if signer := Signer(); signer != nil {
		return signer.SignToken(claims)
	}

	// Legacy path: only for deployments not yet migrated to asymmetric signing.
	if config.AppConfig.AllowLegacyHS256 {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		return token.SignedString([]byte(config.AppConfig.JWTSecret))
	}
	return "", errors.New("this deployment cannot issue tokens (no signing key configured)")
}

// VerifyToken validates an access token. ES256 tokens are verified against the auth
// service's public keys (JWKS or the in-process dev signer); HS256 tokens are accepted
// only while ALLOW_LEGACY_HS256 is on, for the migration window.
func VerifyToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		switch token.Method.Alg() {
		case jwt.SigningMethodES256.Alg():
			return es256Key(token)
		case jwt.SigningMethodHS256.Alg():
			if !config.AppConfig.AllowLegacyHS256 {
				return nil, errors.New("HS256 tokens are no longer accepted")
			}
			return []byte(config.AppConfig.JWTSecret), nil
		default:
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

func es256Key(token *jwt.Token) (interface{}, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("token missing key id")
	}

	// Same-process verification when this service is the signer (auth plane / dev).
	switch s := Signer().(type) {
	case *devSigner:
		if kid == s.kid {
			return s.publicKey(), nil
		}
	case *kmsSigner:
		if pub, ok := s.publicKeyFor(kid); ok {
			return pub, nil
		}
	}

	if globalJWKS == nil {
		return nil, errors.New("JWKS verification not configured")
	}
	return globalJWKS.get(kid)
}
