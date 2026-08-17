package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VerificationCode struct {
	ID         string    `gorm:"primaryKey;type:text"`
	Identifier string    `gorm:"index;not null;type:text"`
	Type       string    `gorm:"not null;type:text"`
	Code       string    `gorm:"index;not null;type:text"`
	ExpiresAt  time.Time `gorm:"not null;type:timestamp with time zone"`
	CreatedAt  time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
	Attempts   int       `gorm:"default:0"`
	Verified   bool      `gorm:"default:false"`
}

func (v *VerificationCode) BeforeCreate(tx *gorm.DB) (err error) {
	if v.ID == "" {
		v.ID = uuid.New().String()
	}
	return
}
