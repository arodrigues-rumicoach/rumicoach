package models

import "time"

// UserAppOpen records that a user opened the app on a given calendar date.
// One row per (user_id, open_date) — unique constraint prevents duplicates.
type UserAppOpen struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	UserID    string    `gorm:"not null;type:text;uniqueIndex:idx_user_open_date"`
	OpenDate  time.Time `gorm:"not null;type:date;uniqueIndex:idx_user_open_date"`
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
}
