package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DailyJourney struct {
	ID          string    `gorm:"primaryKey;type:text"`
	UserID      string    `gorm:"not null;type:text;uniqueIndex:idx_user_date"`
	Date        string    `gorm:"not null;type:text;uniqueIndex:idx_user_date"` // YYYY-MM-DD
	SessionType string    `gorm:"not null;type:text"`
	QuoteID     string    `gorm:"not null;type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
}

func (dg *DailyJourney) BeforeCreate(tx *gorm.DB) (err error) {
	if dg.ID == "" {
		dg.ID = uuid.New().String()
	}
	return
}
