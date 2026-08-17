package services

import (
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVerifyTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Raw DDL (not AutoMigrate) mirrors the repo's other sqlite tests and avoids the
	// driver's timestamp-scan quirk on AutoMigrate-created columns.
	if err := db.Exec(`CREATE TABLE verification_codes (
		id TEXT PRIMARY KEY,
		identifier TEXT NOT NULL,
		type TEXT NOT NULL,
		code TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		attempts INTEGER DEFAULT 0,
		verified NUMERIC DEFAULT 0
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	// The service reads database.Auth(), which falls back to database.DB when AuthDB is nil.
	database.DB = db
	database.AuthDB = nil
	GlobalVerificationService.logger = zap.NewNop()
}

func TestVerifyCode_LocksAfterMaxAttempts(t *testing.T) {
	setupVerifyTestDB(t)
	svc := GlobalVerificationService

	code, _, err := svc.CreateVerificationCode("a@b.com", "email")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// maxVerifyAttempts wrong guesses.
	for i := 0; i < maxVerifyAttempts; i++ {
		if ok, _ := svc.VerifyCode("a@b.com", "email", "000000"); ok {
			t.Fatalf("wrong code accepted on attempt %d", i+1)
		}
	}

	// The code is now locked — even the CORRECT code must be rejected.
	if ok, _ := svc.VerifyCode("a@b.com", "email", code); ok {
		t.Fatal("correct code accepted after attempts were exhausted; brute-force cap not enforced")
	}
}

func TestVerifyCode_CorrectWithinLimitSucceeds(t *testing.T) {
	setupVerifyTestDB(t)
	svc := GlobalVerificationService

	code, _, _ := svc.CreateVerificationCode("a@b.com", "email")

	// A couple of wrong guesses, then the right one — should still work.
	svc.VerifyCode("a@b.com", "email", "111111")
	svc.VerifyCode("a@b.com", "email", "222222")

	if ok, err := svc.VerifyCode("a@b.com", "email", code); !ok {
		t.Fatalf("correct code rejected within attempt limit: ok=%v err=%v", ok, err)
	}

	// A verified code cannot be replayed.
	if ok, _ := svc.VerifyCode("a@b.com", "email", code); ok {
		t.Fatal("verified code accepted a second time")
	}
}

func TestCreateVerificationCode_IssueRateLimit(t *testing.T) {
	setupVerifyTestDB(t)
	svc := GlobalVerificationService

	for i := 0; i < maxCodesPerWindow; i++ {
		if _, _, err := svc.CreateVerificationCode("a@b.com", "email"); err != nil {
			t.Fatalf("issue %d failed: %v", i+1, err)
		}
	}

	// The next request in the window must be refused so attackers can't reset the
	// per-code attempt cap by requesting fresh codes.
	if _, _, err := svc.CreateVerificationCode("a@b.com", "email"); err != ErrTooManyCodeRequests {
		t.Fatalf("expected ErrTooManyCodeRequests, got %v", err)
	}

	// A different identifier is unaffected.
	if _, _, err := svc.CreateVerificationCode("other@b.com", "email"); err != nil {
		t.Fatalf("unrelated identifier rate-limited: %v", err)
	}
}

func TestCreateVerificationCode_ReviewAccountFixedCode(t *testing.T) {
	setupVerifyTestDB(t)
	svc := GlobalVerificationService

	config.AppConfig = &config.Config{
		ReviewAccounts: "yc-review@rumi.coach:424242, apple-review@rumi.coach:171717",
	}
	t.Cleanup(func() { config.AppConfig = nil })

	// Review identifiers get their configured fixed code (case-insensitive match).
	code, _, err := svc.CreateVerificationCode("YC-Review@rumi.coach", "email")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if code != "424242" {
		t.Fatalf("expected fixed review code, got %q", code)
	}
	if ok, err := svc.VerifyCode("YC-Review@rumi.coach", "email", "424242"); !ok {
		t.Fatalf("fixed review code rejected: %v", err)
	}

	// Each review account has its own code.
	if code, _, _ = svc.CreateVerificationCode("apple-review@rumi.coach", "email"); code != "171717" {
		t.Fatalf("expected apple review code, got %q", code)
	}

	// Everyone else still gets random codes.
	code, _, err = svc.CreateVerificationCode("a@b.com", "email")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if code == "424242" {
		t.Fatal("non-review identifier received the review code")
	}

	// A phone identifier never matches the email allowlist.
	code, _, _ = svc.CreateVerificationCode("yc-review@rumi.coach", "phone")
	if code == "424242" {
		t.Fatal("review code issued for a phone-type verification")
	}

	// With the feature unconfigured, the review email is a normal identifier.
	config.AppConfig = &config.Config{}
	code, _, _ = svc.CreateVerificationCode("fresh-review@rumi.coach", "email")
	if code == "424242" {
		t.Fatal("review code issued without configuration")
	}
}
