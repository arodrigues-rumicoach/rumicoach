package services

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"math/big"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Brute-force limits for one-time verification codes. A 6-digit code has 10^6
// combinations; capping guesses per code and codes issued per identifier keeps the
// search space far out of reach within the code's lifetime.
const (
	maxVerifyAttempts = 5 // wrong guesses allowed before a code is locked
	codeValidity      = 15 * time.Minute
	maxCodesPerWindow = 5             // codes that may be issued per identifier ...
	codeIssueWindow   = 1 * time.Hour // ... within this rolling window
)

// ErrTooManyCodeRequests is returned when an identifier has requested too many codes
// in the rolling window. Callers should surface a generic message, not echo the cause.
var ErrTooManyCodeRequests = errors.New("too many verification requests")

type VerificationService struct {
	logger *zap.Logger
}

var GlobalVerificationService = &VerificationService{}

func InitVerificationService(l *zap.Logger) {
	GlobalVerificationService.logger = l
}

func (v *VerificationService) GenerateCode(length int) (string, error) {
	const chars = "0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		result[i] = chars[num.Int64()]
	}
	return string(result), nil
}

func (v *VerificationService) CreateVerificationCode(identifier, reqType string) (string, string, error) {
	db := database.Auth()

	// Rate-limit issuance per identifier: without this an attacker could reset the
	// per-code attempt cap simply by requesting a fresh code between guess batches.
	var recent int64
	db.Model(&models.VerificationCode{}).
		Where("identifier = ? AND type = ? AND created_at > ?", identifier, reqType, time.Now().Add(-codeIssueWindow)).
		Count(&recent)
	if recent >= maxCodesPerWindow {
		return "", "", ErrTooManyCodeRequests
	}

	// Invalidate previous codes
	db.Model(&models.VerificationCode{}).
		Where("identifier = ? AND type = ? AND verified = false AND expires_at > ?", identifier, reqType, time.Now()).
		Update("expires_at", time.Now())

	// Review/demo accounts (App Store review, YC application) get their configured
	// fixed code instead of a random one. The rest of the lifecycle — expiry,
	// attempt caps, issuance rate limit, verified flag — is unchanged, so login and
	// signup flows work identically; delivery is skipped by the handler.
	code := ""
	if reqType == "email" {
		code = config.AppConfig.ReviewAccountCode(identifier)
	}
	if code == "" {
		var err error
		code, err = v.GenerateCode(6)
		if err != nil {
			v.logger.Error("Failed to generate random code", zap.Error(err))
			return "", "", err
		}
	}

	entry := models.VerificationCode{
		Identifier: identifier,
		Type:       reqType,
		Code:       code,
		ExpiresAt:  time.Now().Add(codeValidity),
		Verified:   false,
	}

	if err := db.Create(&entry).Error; err != nil {
		return "", "", err
	}

	return code, entry.ID, nil
}

// VerifyCode checks a submitted code against the latest live code for the identifier.
// Every submission (right or wrong) burns an attempt; once maxVerifyAttempts is reached
// the code is locked (expired) so it can never be brute-forced past that count. The
// code lookup is done by identifier+type only — never by the submitted code — so a
// wrong guess still lands on the same row and increments its counter.
func (v *VerificationService) VerifyCode(identifier, reqType, code string) (bool, error) {
	db := database.Auth()
	var record models.VerificationCode

	err := db.Where("identifier = ? AND type = ? AND expires_at > ? AND verified = false",
		identifier, reqType, time.Now()).
		Order("created_at desc").
		First(&record).Error
	if err != nil {
		return false, err // no live code (never issued, expired, already used, or locked)
	}

	if record.Attempts >= maxVerifyAttempts {
		// Defensive: lock it in case it slipped through the expiry filter.
		db.Model(&record).Update("expires_at", time.Now())
		return false, errors.New("too many attempts")
	}

	// Count this attempt before deciding, so a crash mid-verify can't grant free tries.
	db.Model(&record).Update("attempts", gorm.Expr("attempts + 1"))

	// Constant-time compare so a wrong code can't be discovered digit-by-digit via timing.
	if subtle.ConstantTimeCompare([]byte(record.Code), []byte(code)) != 1 {
		if record.Attempts+1 >= maxVerifyAttempts {
			db.Model(&record).Update("expires_at", time.Now()) // burned all attempts → lock
		}
		return false, errors.New("invalid code")
	}

	db.Model(&record).Update("verified", true)
	return true, nil
}

func (v *VerificationService) ValidateVerificationID(verificationID, identifier, reqType string) bool {
	db := database.Auth()
	var record models.VerificationCode

	cutoff := time.Now().Add(-1 * time.Hour)

	err := db.Where("id = ? AND identifier = ? AND type = ? AND verified = true AND created_at > ?",
		verificationID, identifier, reqType, cutoff).
		First(&record).Error

	return err == nil
}
