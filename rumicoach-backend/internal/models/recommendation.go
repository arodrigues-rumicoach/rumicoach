package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Recommendation struct {
	ID          string    `gorm:"primaryKey;type:text"`
	UserID      string    `gorm:"index;type:text;not null"`
	SessionID   string    `gorm:"index;type:text;not null"`
	Title       string    `gorm:"type:text;not null"`
	Type        string    `gorm:"type:text;not null"` // book, article, video, podcast, other
	Author      *string   `gorm:"type:text"`
	URL         *string   `gorm:"type:text"`
	Description string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
}

func (r *Recommendation) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return
}
