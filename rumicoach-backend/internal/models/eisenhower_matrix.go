package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EisenhowerMatrixExercise struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index;not null;type:text" json:"userId"`
	SessionID string    `gorm:"index;type:text" json:"sessionId"`
	Data      string    `gorm:"type:text" json:"data"` // JSON array of EisenhowerMatrixItem
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
}

func (c *EisenhowerMatrixExercise) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return
}
