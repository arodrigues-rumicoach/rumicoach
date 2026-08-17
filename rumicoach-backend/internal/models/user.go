package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SessionState string

const (
	// --- Onboarding session flow ---
	// ONBOARDING_INTRO folds the old ONBOARDING + ONBOARDING_MEMORIES sub-states.
	StateOnboardingIntro SessionState = "ONBOARDING_INTRO"
	// StateLegacyOnboarding is the pre-rename value folded into ONBOARDING_INTRO. It was
	// also the users.state column default, so every freshly provisioned user carried it —
	// and because no code recognized it, brand-new users fell through to the daily
	// check-in prompt instead of onboarding. A migration backfill normalizes stored rows,
	// but the value must stay recognized wherever states are switched on (rows written
	// before the backfill of a deploy runs).
	StateLegacyOnboarding SessionState = "ONBOARDING"
	// --- Vision session states ---
	// These five used to be the tail of one long onboarding session and carried an
	// ONBOARDING_ prefix. The Vision session now owns them: onboarding is the intro
	// alone, and everything from the ideal-life visualization onwards is its own
	// session. The prefix had to change with the ownership because IsOnboarding()
	// keys off it (balance exemption, journey routing). Stored rows are normalized by
	// backfillLegacyUserStates in database/db.go.
	// VISION_IDEAL_LIFE: the ideal-life visualization.
	StateVisionIdealLife SessionState = "VISION_IDEAL_LIFE"
	// VISION_WHEEL_OF_LIFE folds the old WHEEL_OF_LIFE_CATEGORIES + WHEEL_OF_LIFE sub-states.
	StateVisionWheelOfLife SessionState = "VISION_WHEEL_OF_LIFE"
	// VISION_METAPHOR: focus-area commitment (save_focus).
	StateVisionMetaphor SessionState = "VISION_METAPHOR"
	// VISION_EMOTIONAL_CLOSING / VISION_ENDING_SESSION close out the Vision session.
	StateVisionEmotionalClosing SessionState = "VISION_EMOTIONAL_CLOSING"
	StateVisionEndingSession    SessionState = "VISION_ENDING_SESSION"

	// --- Deep-coaching / check-in sessions ---
	// CHECKIN is the resting state a user holds after completing onboarding. Which deep session
	// (movement, values, ...) or check-in to run next is decided by the journey endpoint from
	// session history, not by this state.
	StateEmotionalClosing SessionState = "EMOTIONAL_CLOSING"
	StateEndingSession    SessionState = "ENDING_SESSION"
	StateCheckin          SessionState = "CHECKIN"

	// --- Planned product sessions (single-state scaffolds for now) ---
	StateMovement   SessionState = "MOVEMENT"
	StateValues     SessionState = "VALUES"
	StateEnergy     SessionState = "ENERGY"
	StateDecisions  SessionState = "DECISIONS"
	StateBeliefs    SessionState = "BELIEFS"
	StateIdentity   SessionState = "IDENTITY"
	StateAcceptance SessionState = "ACCEPTANCE"
	StatePriorities SessionState = "PRIORITIES"
)

// IsEmotionalClosing reports whether the state is an emotional-closing phase
// (the Vision session's own, or the generic one used by other session types).
func (s SessionState) IsEmotionalClosing() bool {
	return s == StateVisionEmotionalClosing || s == StateEmotionalClosing
}

// IsEndingSession reports whether the state is an ending-session phase
// (the Vision session's own, or the generic one used by other session types).
func (s SessionState) IsEndingSession() bool {
	return s == StateVisionEndingSession || s == StateEndingSession
}

// IsOnboarding reports whether the user is still in the onboarding intro — the
// short first session, now just the greeting, privacy explanation and roadmap.
// Covers ONBOARDING_INTRO plus the legacy bare ONBOARDING value.
func (s SessionState) IsOnboarding() bool {
	return strings.HasPrefix(string(s), "ONBOARDING")
}

// NeedsProfileDetails reports whether the details the onboarding intro collects
// (country, date of birth, gender) are still missing. The app starts the intro right
// after account creation; this is how the server can tell an interrupted signup from a
// finished one without trusting the client.
func (u *User) NeedsProfileDetails() bool {
	return u == nil || u.DateOfBirth == nil || u.Gender == nil || *u.Gender == "" ||
		u.Country == nil || *u.Country == ""
}

// IsVision reports whether the user is in the Vision session — the ideal-life
// visualization through to its closing, which the onboarding intro hands over to.
func (s SessionState) IsVision() bool {
	return strings.HasPrefix(string(s), "VISION_")
}

// There was an IsFirstJourney/InFirstJourney pair here that answered "is this user
// still in the free opening pair?" from users.state, and it was what decided the
// balance exemption. It is gone on purpose, so nothing reaches for it again.
//
// The rule was unsound in one direction only, which is why it went unnoticed: state
// defaults to VISION_IDEAL_LIFE on every new account and the sole transition out of
// the VISION_ family is finishing the Vision session cleanly. Anyone who hung up
// part-way through — which is what testing and a dropped call both look like — stayed
// exempt permanently, never gated and never debited.
//
// Its replacement counted rows in communication_sessions, and was unsound in the
// other direction: the row is written when the socket opens, so a connection the user
// dropped after five seconds spent a free session and left them on a paywall in the
// middle of being onboarded.
//
// The allowance is now read from what the opening pair produces — NeedsProfileDetails
// above and users.ideal_life_vision_set_at — with a cap as a backstop: see
// balance.FreeSessionAvailable. Artifacts cannot stall the way a state transition can,
// and cannot be spent by a session that never happened.

// User is the regional profile: everything about a user except credentials and
// login identity, which live in the auth-plane Identity model (rumi_auth database).
// The row exists only in the regional database of the user's chosen data region.
type User struct {
	ID          string     `gorm:"primaryKey;type:text"`
	Email       *string    `gorm:"unique;type:text"`
	PhoneNumber *string    `gorm:"type:text"`
	Name        *string    `gorm:"type:text"`
	DateOfBirth *time.Time `gorm:"type:date"`
	Gender      *string    `gorm:"type:text"`
	CoachGender *string    `gorm:"type:text"`
	Country     *string    `gorm:"type:text"`
	Region      *string    `gorm:"type:text"`
	// DataRegion is the user's chosen data-residency region ("eu" or "us").
	// NULL is treated as "eu": all pre-migration users live in the EU database.
	DataRegion                   *string    `gorm:"type:text"`
	PreferredLanguage            *string    `gorm:"type:text"`
	LastOnlineAt                 *time.Time `gorm:"type:timestamp with time zone"`
	LastPlatform                 *string    `gorm:"type:text"`
	IsActive                     bool       `gorm:"default:false"`
	TermsAndConditionsAcceptedAt *time.Time `gorm:"type:timestamp with time zone"`
	MarketingAcceptedAt          *time.Time `gorm:"type:timestamp with time zone"`
	AIAcceptedAt                 *time.Time `gorm:"type:timestamp with time zone"`
	IdealLifeVision              *string    `gorm:"type:text"`
	IdealLifeVisionSetAt         *time.Time `gorm:"type:timestamp with time zone"`
	FocusArea                    *string    `gorm:"type:text"`
	// TopValues is the outcome of the Values session — the values the user chose as
	// their compass, in their own language, written by save_top_values (replace
	// semantics). Durable personal data: injected into every later session's profile
	// and shown on the Values session's synthesis card.
	TopValues StringSlice `gorm:"type:jsonb"`
	// State is the SessionState the user is parked at. Fresh accounts default to
	// VISION_IDEAL_LIFE, not the intro: the app starts the onboarding intro explicitly
	// right after account creation, so the resting state is what comes AFTER it. The
	// journey still routes an interrupted signup back to the intro — that is decided
	// from session history (journey.ProposeSession), not from this column.
	State *string `gorm:"column:state;type:text;default:'VISION_IDEAL_LIFE'"`
	Theme *string `gorm:"type:text;default:'waterfall'"`
	// JourneyTheme and JourneyQuoteCategory are the AI's end-of-session picks for
	// the coming days' Journey screen (schedule_notifications tool): the theme the
	// frontend should render and the category the daily quote is filtered by.
	// Separate from Theme, which stays the user's manual settings choice.
	JourneyTheme          *string    `gorm:"type:text"`
	JourneyQuoteCategory  *string    `gorm:"type:text"`
	CoachVoice            *string    `gorm:"type:text"`
	LatestSessionHandle   *string    `gorm:"type:text"`
	LatestSessionHandleAt *time.Time `gorm:"type:timestamp with time zone"`
	// ChatHistoryRetentionDays is how long the WhatsApp/Telegram conversation is kept.
	// 0 means keep it indefinitely — an explicit choice, never a default, because it gives
	// up the only guarantee this setting exists to provide. The value is materialized onto
	// every message as ExpiresAt, so it must only ever be changed through
	// SetChatHistoryRetention, which rewrites those in the same transaction.
	ChatHistoryRetentionDays int `gorm:"not null;default:7"`
	// JourneyResetAt is when the user last erased their progress. Session rows are never
	// deleted — they carry duration and token analytics — so this is the only thing that
	// separates "this happened" from "this counts towards where I am now". Everything that
	// reads progress out of communication_sessions goes through models.JourneySessions,
	// which filters on it. Nil means the whole history counts.
	JourneyResetAt *time.Time `gorm:"type:timestamp with time zone"`
	// BalanceSeconds is the user's remaining balance in seconds. Kept in sync with
	// balance_transactions inside the same DB transaction (see internal/services/
	// balance); may go negative.
	//
	// <-:false makes the field read-only to GORM's struct writes: a full-row Save
	// anywhere in the codebase silently skips this column instead of clobbering a
	// concurrent debit with a stale value. Three call sites used to carry
	// Omit("balance_seconds") to defend against exactly that, protected by nothing
	// but a comment; now the model enforces it. The only writers are raw-SQL
	// statements that say what they are doing: the ledger's apply() and the QA
	// account reset (which deletes the ledger in the same transaction).
	// AutoMigrate still creates the column — write permission is DML-only.
	BalanceSeconds int64      `gorm:"not null;default:0;<-:false"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;type:timestamp with time zone"`
	DeletedAt      *time.Time `gorm:"type:timestamp with time zone"`
	// Waitlisted marks a User view synthesized from a pending waitlist identity
	// (no regional profile row yet); never stored, admin listing only.
	Waitlisted bool `gorm:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return
}

// ChatRetentionOptions are the values the API accepts for ChatHistoryRetentionDays. A
// closed set rather than a free integer: it keeps the app's picker and the server in
// agreement, and stops anyone storing a retention of 3650 days that reads as a policy but
// behaves as "forever".
//
// Nothing below 3 days is offered. The companion only replays the last 72 hours of
// conversation, so a shorter retention would leave it losing the thread mid-exchange with
// nothing on screen to explain why.
var ChatRetentionOptions = []int{0, 3, 7, 30}

// ValidChatRetention reports whether days is one of the offered values.
func ValidChatRetention(days int) bool {
	for _, v := range ChatRetentionOptions {
		if v == days {
			return true
		}
	}
	return false
}

// ChatMessageExpiry returns when a message created at createdAt should be deleted under a
// retention of days, or nil when the user chose to keep their history (0).
func ChatMessageExpiry(createdAt time.Time, days int) *time.Time {
	if days <= 0 {
		return nil
	}
	t := createdAt.AddDate(0, 0, days)
	return &t
}

// ResidencyRegion returns the user's data-residency region code, defaulting to
// "eu" for users created before region selection existed.
func (u *User) ResidencyRegion() string {
	if u.DataRegion != nil && *u.DataRegion != "" {
		return *u.DataRegion
	}
	return "eu"
}

type UserDevice struct {
	ID        string    `gorm:"primaryKey;type:text"`
	UserID    string    `gorm:"index;type:text;not null"`
	FCMToken  string    `gorm:"uniqueIndex;type:text;not null"`
	Platform  string    `gorm:"type:text"` // e.g., "ios", "android", "web"
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone"`
}

func (d *UserDevice) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return
}
