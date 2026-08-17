package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Channel providers.
const (
	ChannelProviderWhatsApp = "whatsapp"
	ChannelProviderTelegram = "telegram"
)

// Integration statuses.
const (
	IntegrationPending = "pending" // link code issued, waiting for the user's first message
	IntegrationActive  = "active"
	IntegrationRevoked = "revoked"
)

// Integration reply modes.
const (
	ChannelReplyModeText  = "text"
	ChannelReplyModeAudio = "audio"
	ChannelReplyModeAuto  = "auto"
)

// Integration ties a user account to an external messaging identity (e.g. a
// WhatsApp phone number). A binding starts as pending with a short-lived link
// code that the app hands to the user as a wa.me deep link; when the user sends
// the code from their phone the binding activates and ExternalID is filled in.
type Integration struct {
	ID       string `gorm:"primaryKey;type:text"`
	UserID   string `gorm:"index;not null;type:text"`
	Provider string `gorm:"not null;type:text;uniqueIndex:idx_channel_provider_external"`
	// ExternalID is the provider-side identity (E.164 phone for WhatsApp,
	// chat id for Telegram). Nil while the binding is pending.
	ExternalID *string `gorm:"type:text;uniqueIndex:idx_channel_provider_external"`
	Status     string  `gorm:"not null;type:text;default:'pending'"`
	// LinkCode is the one-time code (e.g. "RUMI-7K3M2X"); cleared on activation.
	LinkCode          *string    `gorm:"uniqueIndex;type:text"`
	LinkCodeExpiresAt *time.Time `gorm:"type:timestamp with time zone"`
	// ReplyMode selects whether Rumi answers with text or voice notes.
	ReplyMode string `gorm:"not null;type:text;default:'text'"`
	// LastInboundAt tracks the most recent user message, which defines the
	// provider's customer-service window (WhatsApp: 24h) for free-form sends.
	LastInboundAt *time.Time `gorm:"type:timestamp with time zone"`
	// LastOutboundAt mirrors LastInboundAt for messages WE send, and is what gates
	// unprompted sends (see the companion's mayReachOutNow). It lives here, and not as a
	// COUNT over channel_messages, because the message log is erasable: a user who
	// cleared their chat for privacy could be messaged again immediately — the opposite
	// of what they asked for — and clearing it deliberately reset the guard.
	LastOutboundAt *time.Time `gorm:"type:timestamp with time zone"`
	// DailyInboundCount and DailyInboundDate carry the daily inbound-message cap for the
	// same reason: derived from the log, the cap reset the moment the log was cleared,
	// which made "delete my chat" an abuse path. The count rolls over lazily — whenever
	// DailyInboundDate is not today in UTC, it restarts at zero.
	DailyInboundCount int        `gorm:"not null;default:0"`
	DailyInboundDate  *time.Time `gorm:"type:date"`
	CreatedAt         time.Time  `gorm:"autoCreateTime;type:timestamp with time zone"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime;type:timestamp with time zone"`
}

// utcDay is the UTC calendar day of t, the granularity DailyInboundDate stores.
func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// RollDailyInbound advances the daily inbound counter for a message received at now,
// restarting it when the stored date is not today. Returns the new count. It only
// mutates the struct; persisting is the caller's job, so the write can be batched with
// LastInboundAt.
func (i *Integration) RollDailyInbound(now time.Time) int {
	today := utcDay(now)
	// Compare as calendar dates: the column is a DATE, and what comes back from the
	// driver (Postgres date, SQLite text) cannot be trusted to be an exact midnight.
	if i.DailyInboundDate == nil || i.DailyInboundDate.UTC().Format("2006-01-02") != today.Format("2006-01-02") {
		i.DailyInboundCount = 0
		i.DailyInboundDate = &today
	}
	i.DailyInboundCount++
	return i.DailyInboundCount
}

// MayReachOutAfter reports whether an unprompted message may be sent now, i.e. no
// outbound message has gone out within quiet.
func (i *Integration) MayReachOutAfter(quiet time.Duration) bool {
	return i.LastOutboundAt == nil || time.Since(*i.LastOutboundAt) >= quiet
}

func (i *Integration) BeforeCreate(tx *gorm.DB) (err error) {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return
}

// InsideServiceWindow reports whether a free-form (non-template) message may be
// sent, i.e. the user messaged us within the given window (24h for WhatsApp).
func (i *Integration) InsideServiceWindow(window time.Duration) bool {
	return i.LastInboundAt != nil && time.Since(*i.LastInboundAt) < window
}

// CanSendFreeForm reports whether a free-form (non-template) message may be sent
// on this integration channel. WhatsApp strictly enforces a 24h customer-service
// window outside of which only pre-approved templates can be sent. Other channels
// (such as Telegram) do not have customer-service window restrictions.
func (i *Integration) CanSendFreeForm(window time.Duration) bool {
	if i.Provider == ChannelProviderWhatsApp {
		return i.InsideServiceWindow(window)
	}
	return true
}
