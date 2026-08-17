package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AcceptanceReflection is the structured outcome of the Expectations & Acceptance
// session's synthesis (Phase 9, Filipa's "Acceptance Reflection Card"): what the user
// expected, what is true right now, what they cannot control, what they can influence,
// what they choose to accept, where they choose to act, and (optionally) their next
// step. One row per session — a correction replaces the previous capture, mirroring
// save_identity_reflection.
//
// All text fields are in the user's language, in the USER'S first-person voice ("I
// expected my partner to understand what I needed without me having to ask") — they are
// rendered verbatim on the session-end card.
type AcceptanceReflection struct {
	ID        string `gorm:"primaryKey;type:text" json:"id"`
	UserID    string `gorm:"index;type:text;not null" json:"userId"`
	SessionID string `gorm:"index;type:text;not null" json:"sessionId"`

	// Expected — WHAT I EXPECTED.
	Expected string `gorm:"type:text;not null" json:"expected"`
	// Reality — WHAT IS TRUE RIGHT NOW.
	Reality string `gorm:"type:text;not null" json:"reality"`
	// CannotControl — WHAT I CAN'T CONTROL.
	CannotControl string `gorm:"type:text" json:"cannotControl"`
	// CanInfluence — WHAT I CAN INFLUENCE.
	CanInfluence string `gorm:"type:text" json:"canInfluence"`
	// ChooseToAccept — WHAT I CHOOSE TO ACCEPT.
	ChooseToAccept string `gorm:"type:text" json:"chooseToAccept"`
	// WhereIAct — WHERE I CHOOSE TO ACT.
	WhereIAct string `gorm:"type:text" json:"whereIAct"`
	// NextStep — NEXT STEP, optional (Phase 8's commitment is optional by design:
	// waiting, observing, or doing nothing for now are all valid outcomes).
	NextStep string `gorm:"type:text" json:"nextStep"`

	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (r *AcceptanceReflection) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return
}
