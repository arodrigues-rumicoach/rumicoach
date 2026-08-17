package auth

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/rumi/rumi-be/config"
)

func initTestSigner(t *testing.T) {
	t.Helper()
	config.LoadConfig()
	config.AppConfig.Environment = "development"
	config.AppConfig.AuthKMSKeyID = ""
	if err := InitSigner(context.Background()); err != nil {
		t.Fatalf("InitSigner: %v", err)
	}
}

func TestES256SignVerifyRoundtrip(t *testing.T) {
	initTestSigner(t)

	token, err := CreateAccessToken("user-1", "u@example.com", "U", true, true, false, false, "us")
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	claims, err := VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("subject = %q, want user-1", claims.Subject)
	}
	if claims.Region != "us" {
		t.Errorf("region = %q, want us", claims.Region)
	}
}

func TestJWKSServed(t *testing.T) {
	initTestSigner(t)

	rec := httptest.NewRecorder()
	JWKSHandler(rec, httptest.NewRequest("GET", "/.well-known/jwks.json", nil))
	if rec.Code != 200 {
		t.Fatalf("JWKS status = %d", rec.Code)
	}
	if body := rec.Body.String(); len(body) == 0 || body[0] != '{' {
		t.Errorf("unexpected JWKS body: %s", body)
	}
}

func TestRegionEnforcement(t *testing.T) {
	initTestSigner(t)

	cases := []struct {
		planeRegion string
		tokenRegion string
		allowed     bool
	}{
		{"", "us", true},    // no enforcement configured (dev)
		{"eu", "eu", true},  // matching region
		{"us", "us", true},  // matching region
		{"us", "eu", false}, // EU user on US plane
		{"eu", "us", false}, // US user on EU plane
		{"eu", "", true},    // legacy token without region claim → EU
		{"us", "", false},   // legacy token is never a US resident
	}

	for _, c := range cases {
		config.AppConfig.RegionCode = c.planeRegion
		got := RegionAllowed(&Claims{Region: c.tokenRegion})
		if got != c.allowed {
			t.Errorf("plane=%q token=%q: allowed=%v, want %v", c.planeRegion, c.tokenRegion, got, c.allowed)
		}
	}
	config.AppConfig.RegionCode = ""
}

func TestHS256RejectedWhenLegacyDisabled(t *testing.T) {
	initTestSigner(t)

	// Issue a legacy HS256 token by disabling the signer.
	saved := globalSigner
	globalSigner = nil
	config.AppConfig.AllowLegacyHS256 = true
	legacy, err := CreateAccessToken("user-2", "", "", true, true, false, false, "eu")
	globalSigner = saved
	if err != nil {
		t.Fatalf("legacy CreateAccessToken: %v", err)
	}

	if _, err := VerifyToken(legacy); err != nil {
		t.Fatalf("legacy token should verify during migration window: %v", err)
	}

	config.AppConfig.AllowLegacyHS256 = false
	defer func() { config.AppConfig.AllowLegacyHS256 = true }()
	if _, err := VerifyToken(legacy); err == nil {
		t.Error("legacy HS256 token verified with ALLOW_LEGACY_HS256=false")
	}
}
