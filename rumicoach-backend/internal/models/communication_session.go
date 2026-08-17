package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Token counts used to live here (input/output/total plus a per-modality breakdown).
// They moved to ai_usage_records, the product-wide cost ledger, so that a voice
// session, a companion message and a background generation can be summed together —
// which columns on this table could never do. The DB columns still hold their history;
// nothing writes or reads them any more.
type CommunicationSession struct {
	ID                 string     `gorm:"primaryKey;type:text"`
	UserID             string     `gorm:"index;not null;type:text"`
	StartTime          time.Time  `gorm:"not null;type:timestamp with time zone"`
	EndTime            *time.Time `gorm:"type:timestamp with time zone"`
	Duration           int        `gorm:"default:0"`
	Language           *string    `gorm:"type:text"`
	DeepgramDuration   float64    `gorm:"default:0.0"`
	STTService         *string    `gorm:"type:text"`
	SessionType        *string    `gorm:"type:text"`
	Transcript         *string    `gorm:"type:text"`
	AINotes            *string    `gorm:"type:text"`
	AIEvaluation       *float64   `gorm:"type:numeric"`
	UserEvaluation     *float64   `gorm:"type:numeric"`
	UserFeedback       *string    `gorm:"type:text"`
	UserSessionInsight *string    `gorm:"type:text"`
	// SessionSummary holds the end-of-session synthesis screen payload (JSON) so the app
	// can re-render it whenever the user revisits the session.
	SessionSummary *string `gorm:"type:text" json:"sessionSummary,omitempty"`
	// Recap is a short, user-facing summary of what the session was about and what the
	// user took from it, generated from the transcript after the session ends and written
	// in the user's own language. It is what the sessions list shows, so it is deliberately
	// brief. Distinct from SessionSummary (the structured synthesis screen) and from
	// AINotes (the QA rubric, which is English and never shown to the user).
	Recap *string `gorm:"type:text" json:"recap,omitempty"`
	// RecapTitle is a handful of words naming what the session was about, generated
	// alongside Recap and in the same language. The sessions list uses it as the row
	// heading, which is why it exists instead of the app labelling every row with the
	// session type.
	RecapTitle *string `gorm:"type:text" json:"recapTitle,omitempty"`
}

func (s *CommunicationSession) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return
}

// JourneySessions scopes a communication_sessions query to the user's CURRENT journey:
// everything since they last erased their progress, or their whole history if they never
// did.
//
// Erasing progress does not delete these rows — they carry the duration and token
// analytics the business runs on — so this filter is the only thing that makes a reset
// real. It exists as one helper rather than a condition copied into each caller because
// progress is read out of this table from a dozen places: the journey, the streak,
// the badges, the check-in prompt, the companion's context. A reset that misses one of
// them is worse than no reset, because the user then sees a streak of zero next to a coach
// opening with "in your last session…".
//
// Deliberately NOT applied to the plain history views (the sessions list, the usage
// calendar): the user is still allowed to see that those sessions happened. What resets is
// what counts, not what is on the record.
//
// resetAt is passed in rather than looked up from a correlated subquery on purpose. The
// subquery version coupled every session read to the users table, which meant each caller
// silently acquired a join and every test harness that touches sessions had to grow a
// users table to match. Callers that already hold the user pass user.JourneyResetAt for
// free; the rest use JourneyStartFor.
func JourneySessions(db *gorm.DB, userID string, resetAt *time.Time) *gorm.DB {
	q := db.Where("user_id = ?", userID)
	if resetAt != nil {
		// test-seed-* rows bypass the cutoff: QA personas seed backdated histories
		// (a Movement session "4 days ago") while the test-setup reset stamps
		// journey_reset_at = now to fence off the tester's REAL previous sessions —
		// without the exemption the stamp would fence off the seeds too and every
		// backdated persona would present as a blank journey. Real users never carry
		// this ID prefix (only POST /v1/admin/test-setup writes it).
		q = q.Where("(start_time > ? OR id LIKE 'test-seed-%')", *resetAt)
	}
	return q
}

// JourneyStartFor loads the journey cutoff for callers that only have a user id. Pass the
// root *gorm.DB, not a query already scoped to another model.
//
// A failed lookup returns nil, meaning the whole history counts. That direction is
// deliberate: a transient database error must not make somebody's streak and journey
// appear erased when nobody asked for it.
func JourneyStartFor(db *gorm.DB, userID string) *time.Time {
	var u User
	if err := db.Model(&User{}).Select("journey_reset_at").
		Where("id = ?", userID).First(&u).Error; err != nil {
		return nil
	}
	return u.JourneyResetAt
}
