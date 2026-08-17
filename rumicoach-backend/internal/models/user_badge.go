package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserBadge struct {
	ID        string    `gorm:"primaryKey;type:text"`
	UserID    string    `gorm:"index;not null;type:text"`
	BadgeType string    `gorm:"index;not null;type:text"`
	EarnedAt  time.Time `gorm:"not null;type:timestamp with time zone"`
}

func (b *UserBadge) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return
}
