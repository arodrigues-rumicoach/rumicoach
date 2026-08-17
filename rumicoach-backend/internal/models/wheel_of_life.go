package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WheelOfLifeExercise struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index;not null;type:text" json:"userId"`
	SessionID string    `gorm:"index;type:text" json:"sessionId"`
	Data      string    `gorm:"type:text" json:"data"` // JSON array of WheelOfLifeItem
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (w *WheelOfLifeExercise) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return
}
