package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StringSlice is a []string persisted as a JSONB column (used for the identity
// reflection's chosen qualities).
type StringSlice []string

// Scan implements sql.Scanner.
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return errors.New("failed to scan StringSlice: unsupported type")
		}
		bytes = []byte(str)
	}
	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// IdentityReflection is the structured outcome of the Identity session's synthesis
// (Phase 10, Filipa's "Identity Reflection Card"): who the user learned to be, what that
// gave them, what it costs, who they choose to keep becoming, the qualities they want to
// strengthen and (optionally) their next piece of evidence. One row per session — a
// correction replaces the previous capture, mirroring save_vision_commitment.
//
// All text fields are in the user's language, in the USER'S first-person voice ("Someone
// who handles everything alone" / "Alguém que resolve tudo sozinha") — they are rendered
// verbatim on the session-end card and re-read by future sessions.
type IdentityReflection struct {
	ID        string `gorm:"primaryKey;type:text" json:"id"`
	UserID    string `gorm:"index;type:text;not null" json:"userId"`
	SessionID string `gorm:"index;type:text;not null" json:"sessionId"`

	// LearnedIdentity — WHO I'VE LEARNED TO BE ("Someone who handles everything alone.")
	LearnedIdentity string `gorm:"type:text;not null" json:"learnedIdentity"`
	// WhatItGave — WHAT THAT HAS GIVEN ME ("Independence and resilience.")
	WhatItGave string `gorm:"type:text" json:"whatItGave"`
	// WhatItCosts — WHAT IT SOMETIMES COSTS ME ("Difficulty asking for help.")
	WhatItCosts string `gorm:"type:text" json:"whatItCosts"`
	// WhoBecoming — WHO I WANT TO KEEP BECOMING ("Independent enough to stand on my
	// own, but secure enough to let others in.")
	WhoBecoming string `gorm:"type:text;not null" json:"whoBecoming"`
	// Qualities — QUALITIES I WANT TO STRENGTHEN, two or three ("Openness", "Courage").
	Qualities StringSlice `gorm:"type:jsonb" json:"qualities"`
	// Evidence — MY NEXT EVIDENCE, optional (the Phase 9 commitment is optional by design).
	Evidence string `gorm:"type:text" json:"evidence"`

	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (r *IdentityReflection) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return
}
