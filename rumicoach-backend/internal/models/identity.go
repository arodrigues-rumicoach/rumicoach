package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Identity is the auth-plane record of an account: who can log in, with which
// credentials, and where their data lives. It is stored in the dedicated rumi_auth
// database (AUTH_DATABASE_URL), never in the regional data-plane databases, so a
// future standalone auth service can take it over as-is.
//
// Everything else about a user (profile, coaching data) lives as a User row in the
// regional database of their chosen data region, under the same ID.
type Identity struct {
	ID          string  `gorm:"primaryKey;type:text"`
	Email       *string `gorm:"unique;type:text"`
	PhoneNumber *string `gorm:"type:text"`
	// Name is kept here for token claims and transactional email greetings.
	Name     *string `gorm:"type:text"`
	GoogleID *string `gorm:"unique;type:text"`
	// AppleID is the stable `sub` claim from Sign in with Apple identity tokens.
	// It is the only reliable key for Apple accounts: the email may be a
	// per-app private relay address and the name is never repeated after the
	// first authorization.
	AppleID *string `gorm:"unique;type:text"`
	// DataRegion is the user's chosen data-residency region ("eu" or "us").
	DataRegion            *string    `gorm:"type:text"`
	Platform              *string    `gorm:"type:text"`
	LastOnlineAt          *time.Time `gorm:"type:timestamp with time zone"`
	IsActive              bool       `gorm:"default:false"`
	IsAdmin               bool       `gorm:"default:false"`
	IsEmailVerified       bool       `gorm:"default:false"`
	IsPhonenumberVerified bool       `gorm:"default:false"`
	RefreshTokenHash      *string    `gorm:"type:text"`
	RefreshTokenExpiresAt *time.Time `gorm:"type:timestamp with time zone"`
	// Consent timestamps are account-level legal records and belong with the identity.
	TermsAndConditionsAcceptedAt *time.Time `gorm:"type:timestamp with time zone"`
	MarketingAcceptedAt          *time.Time `gorm:"type:timestamp with time zone"`
	AIAcceptedAt                 *time.Time `gorm:"type:timestamp with time zone"`
	// PendingProvision holds the JSON-serialized regional.UserProvision for a
	// waitlist signup whose profile has NOT been provisioned in a data plane yet.
	// Provisioning is deferred to account activation; the column is cleared once
	// the profile row exists. NULL means the profile is (or was) provisioned.
	PendingProvision *string    `gorm:"type:text"`
	CreatedAt        time.Time  `gorm:"autoCreateTime;type:timestamp with time zone"`
	DeletedAt        *time.Time `gorm:"type:timestamp with time zone"`
}

func (i *Identity) BeforeCreate(tx *gorm.DB) (err error) {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return
}

// ResidencyRegion returns the identity's data-residency region code, defaulting to "eu".
func (i *Identity) ResidencyRegion() string {
	if i.DataRegion != nil && *i.DataRegion != "" {
		return *i.DataRegion
	}
	return "eu"
}
