package models

import (
	"time"

	"gorm.io/gorm"
)

type Lead struct {
	ID        string         `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Name      string         `json:"name"`
	Email     string         `json:"email"`
	Phone     *string        `json:"phone,omitempty"`
	Company   string         `json:"company"`
	Country   *string        `json:"country,omitempty"`
	Size      *string        `json:"size,omitempty"`
	Message   *string        `json:"message,omitempty"`
	Origin    *string        `json:"origin,omitempty"`
	Language  string         `json:"language" gorm:"default:'en'"`
	State     string         `json:"state" gorm:"default:'NEW'"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}
