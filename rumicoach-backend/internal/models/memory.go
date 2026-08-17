package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserMemory struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index;not null;type:text" json:"userId"`
	Category  string    `gorm:"not null;type:text" json:"category"`
	Content   string    `gorm:"not null;type:text" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
}

func (m *UserMemory) BeforeCreate(tx *gorm.DB) (err error) {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return
}
