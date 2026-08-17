package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CommitmentCompletion tracks daily completion status for recurring commitments.
// If a record exists for a given user, commitment, and date, it means the user marked it as "done" for that day.
type CommitmentCompletion struct {
	ID           string    `gorm:"primaryKey;type:text" json:"id"`
	UserID       string    `gorm:"index:idx_user_date;type:text;not null" json:"userId"`
	CommitmentID string    `gorm:"index:idx_commitment_date;type:text;not null" json:"commitmentId"`
	Date         string    `gorm:"index:idx_user_date;index:idx_commitment_date;type:text;not null" json:"date"`
	CreatedAt    time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (c *CommitmentCompletion) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return
}
