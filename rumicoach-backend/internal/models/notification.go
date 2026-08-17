package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Notification delivery channels (SentVia values). Messaging channels use
// their provider name (ChannelProviderWhatsApp, ...); push is the FCM fallback.
const (
	NotificationChannelPush = "push"
)

// Notification is a scheduled coaching message generated at the end of a live
// session (schedule_notifications tool). It is channel-agnostic: the delivery
// channel is decided at send time by the notification dispatcher — an active
// messaging channel (WhatsApp today, Signal/Telegram later) wins, FCM push is
// the last resort — and exactly one channel is used, recorded in SentVia.
type Notification struct {
	ID          string    `gorm:"primaryKey;type:text"`
	UserID      string    `gorm:"index;type:text;not null"`
	SessionID   string    `gorm:"index;type:text"`
	Title       string    `gorm:"type:text;not null"`
	Message     string    `gorm:"type:text;not null"`
	DelayHours  int       `gorm:"type:integer;not null"`
	ScheduledAt time.Time `gorm:"index;type:timestamp with time zone"`
	// TimeSensitive marks a message whose value is bound to the moment it was scheduled
	// for — tied to something happening in the user's life. These are never shifted to
	// make room for other messages; if the moment passes they are dropped, because
	// "good luck this morning" arriving in the afternoon is worse than silence.
	TimeSensitive bool       `gorm:"not null;default:false"`
	SentAt        *time.Time `gorm:"type:timestamp with time zone"`
	// SentVia records the channel that actually delivered the notification
	// ("whatsapp", "push", ...). Nil until sent.
	SentVia   *string    `gorm:"type:text"`
	FailedAt  *time.Time `gorm:"type:timestamp with time zone"`
	CreatedAt time.Time  `gorm:"autoCreateTime;type:timestamp with time zone"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime;type:timestamp with time zone"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) (err error) {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if n.ScheduledAt.IsZero() {
		n.ScheduledAt = time.Now().Add(time.Duration(n.DelayHours) * time.Hour)
	}
	return
}
