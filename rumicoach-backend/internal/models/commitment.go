package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Commitment origins. An commitment is either part of the coaching plan (created by a session via
// update_commitment_plan/save_commitments) or was added manually by the user (e.g. during a check-in).
// New origins can be added here as the product grows.
const (
	CommitmentOriginPlan   = "plan"
	CommitmentOriginManual = "manual"
	// CommitmentOriginBehavior marks the recurring commitment projected from a BehaviorPlan — the
	// daily execution surface of a habit. The plan owns the why (identity, trigger,
	// plan B); the commitment owns the what/when and rides the existing DailyJourney tracking.
	CommitmentOriginBehavior = "behavior"
)

// IntSlice is a []int persisted as a JSONB column (used for a recurring commitment's weekdays).
type IntSlice []int

// Scan implements sql.Scanner.
func (s *IntSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return errors.New("failed to scan IntSlice: unsupported type")
		}
		bytes = []byte(str)
	}
	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer.
func (s IntSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Commitment is a first-class, standalone unit of commitment tied to the user's focus area (users.focus_area).
// Per-day completion of recurring commitments lives in the DailyJourney snapshot; Done here
// is the master flag (chiefly meaningful for one_time commitments).
type Commitment struct {
	ID     string   `gorm:"primaryKey;type:text" json:"id"`
	UserID string   `gorm:"index;type:text;not null" json:"userId"`
	Origin string   `gorm:"type:text;not null;default:manual" json:"origin"`
	Title  string   `gorm:"type:text;not null" json:"title"`
	Type   string   `gorm:"type:text;not null" json:"type"` // "one_time" | "recurring"
	Days   IntSlice `gorm:"type:jsonb" json:"days,omitempty"`
	Date   *string  `gorm:"type:text" json:"date,omitempty"`
	Done   bool     `gorm:"not null;default:false" json:"done"`
	// EndedAt closes a recurring commitment: it stops generating occurrences from this
	// day on, but the row survives so the history of what was already done (or missed)
	// is not destroyed. Deleting a recurring commitment sets this instead of removing
	// the row. Nil means it is still running — and a recurring commitment with no end
	// runs indefinitely, which is why history is bounded by CreatedAt on the other side.
	EndedAt   *time.Time `gorm:"type:timestamp with time zone" json:"endedAt,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (t *Commitment) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return
}

// ActiveActionsForDate filters a master commitment list down to the commitments that are active on the
// given local date (dateStr, "YYYY-MM-DD") and ISO weekday (1 = Monday … 7 = Sunday). This is the
// single source of truth for the "commitments for today" rule, shared by the journey endpoint and
// the chat daily-sync:
//   - recurring → active when isoWeekday is in Days.
//   - one_time  → active when Date == dateStr, or it is overdue (Date < dateStr) and not yet done.
func ActiveActionsForDate(commitments []Commitment, dateStr string, isoWeekday int) []Commitment {
	var active []Commitment
	for _, t := range commitments {
		switch t.Type {
		case "recurring":
			for _, d := range t.Days {
				if d == isoWeekday {
					active = append(active, t)
					break
				}
			}
		case "one_time":
			if t.Date == nil {
				continue
			}
			// A pending one-time commitment stays visible every day — before its date
			// (upcoming, QA: a check-up scheduled for next week must not vanish from the
			// Journey screen), on it, and after it (overdue). Once done it only shows on
			// its own date.
			if *t.Date == dateStr || !t.Done {
				active = append(active, t)
			}
		}
	}
	return active
}
