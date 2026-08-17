package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChannelFollowUp kinds.
const (
	ChannelFollowUpDailyNudge  = "daily_nudge"
	ChannelFollowUpPostSession = "post_session"
	// ChannelFollowUpScheduledNotification rows are log-only: a scheduled
	// notification composed into chat is not queued here (the notification
	// dispatcher owns its delivery), but the sent text is recorded as an
	// already-drained row so later proactive generations can see it.
	ChannelFollowUpScheduledNotification = "scheduled_notification"
)

// ChannelFollowUp is a queued proactive message for a messaging channel
// (deliberately separate from Notification, whose channel is chosen at send
// time by the notification dispatcher). Rows are
// drained by the companion dispatcher with FOR UPDATE SKIP LOCKED, so multiple
// replicas can run the loop safely; SentAt/FailedAt mark terminal states.
type ChannelFollowUp struct {
	ID        string `gorm:"primaryKey;type:text"`
	UserID    string `gorm:"index;not null;type:text"`
	BindingID string `gorm:"index;not null;type:text"`
	Kind      string `gorm:"not null;type:text"`
	// PayloadHint carries kind-specific context, e.g. the planned session type
	// for a daily nudge.
	PayloadHint *string    `gorm:"type:text"`
	ScheduledAt time.Time  `gorm:"index;not null;type:timestamp with time zone"`
	SentAt      *time.Time `gorm:"type:timestamp with time zone"`
	FailedAt    *time.Time `gorm:"type:timestamp with time zone"`
	// SentText is the message as actually delivered. It is the anti-repetition
	// memory for proactive generation: channel_messages cannot serve that role
	// because the ephemeral purge erases a conversation after a few quiet hours —
	// and quiet users are precisely the ones proactive messages go to. Because
	// this column holds delivered copy, the 30-day follow-up retention sweep and
	// the chat-scope erasure (both of which delete these rows) are load-bearing
	// for privacy, not just hygiene.
	SentText  *string   `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone"`
}

func (f *ChannelFollowUp) BeforeCreate(tx *gorm.DB) (err error) {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return
}
