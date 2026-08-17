package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChannelMessage directions.
const (
	ChannelMessageInbound  = "inbound"
	ChannelMessageOutbound = "outbound"
)

// ChannelMessage types.
const (
	ChannelMessageTypeText     = "text"
	ChannelMessageTypeAudio    = "audio"
	ChannelMessageTypeTemplate = "template"
	ChannelMessageTypeSystem   = "system"
)

// ChannelMessage statuses.
const (
	ChannelMessageReceived  = "received"
	ChannelMessageProcessed = "processed"
	ChannelMessageSent      = "sent"
	ChannelMessageFailed    = "failed"
)

// ChannelMessage is one inbound or outbound message on an external messaging
// channel. The unique ProviderMessageID doubles as the webhook dedupe key:
// providers (Meta in particular) redeliver events, so inserts use ON CONFLICT
// DO NOTHING and a zero-rows result means the event was already handled.
type ChannelMessage struct {
	ID        string `gorm:"primaryKey;type:text"`
	BindingID string `gorm:"index;not null;type:text"`
	UserID    string `gorm:"index;not null;type:text"`
	Provider  string `gorm:"not null;type:text"`
	Direction string `gorm:"not null;type:text"`
	// ProviderMessageID is the provider's message id (WhatsApp "wamid.…").
	ProviderMessageID *string `gorm:"uniqueIndex;type:text"`
	Type              string  `gorm:"not null;type:text"`
	// Body holds the text content; for audio messages it is filled with the
	// transcript once transcription completes.
	Body    *string `gorm:"type:text"`
	MediaID *string `gorm:"type:text"`
	Status  string  `gorm:"not null;type:text"`
	// ExpiresAt is when the retention sweep deletes this row, materialized from the user's
	// ChatHistoryRetentionDays when the message is stored. NULL means it is kept
	// indefinitely, which only happens when the user chose that explicitly.
	//
	// It is written on EVERY insert path — both webhooks and recordOutbound — and rewritten
	// for the whole conversation whenever the setting changes. A row that reaches the table
	// with NULL by accident is never deleted, so the stamping is not optional.
	ExpiresAt *time.Time `gorm:"index;type:timestamp with time zone"`
	CreatedAt time.Time  `gorm:"autoCreateTime;index;type:timestamp with time zone"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime;type:timestamp with time zone"`
}

func (m *ChannelMessage) BeforeCreate(tx *gorm.DB) (err error) {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return
}
