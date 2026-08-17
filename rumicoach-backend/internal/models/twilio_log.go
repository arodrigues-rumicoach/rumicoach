package models

import "time"

type TwilioLog struct {
	ID        uint   `gorm:"primaryKey"`
	From      string `gorm:"type:varchar(255)"`
	To        string `gorm:"type:varchar(255)"`
	Body      string `gorm:"type:text"`
	CreatedAt time.Time
}
