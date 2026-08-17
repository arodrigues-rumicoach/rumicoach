package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Behavior plan statuses. A plan is the coaching layer over a habit — the identity, trigger,
// and adjustment history — never a checklist. Execution lives in the linked recurring Task.
const (
	// BehaviorPlanStatusActive — currently being practiced and followed up in sessions.
	BehaviorPlanStatusActive = "active"
	// BehaviorPlanStatusParked — the user's readiness was low (<7) or they chose to pause;
	// kept so Rumi can gently reactivate it later. Never framed as a failure.
	BehaviorPlanStatusParked = "parked"
	// BehaviorPlanStatusGraduated — the behavior became part of who the user is; no longer
	// followed up, celebrated as identity.
	BehaviorPlanStatusGraduated = "graduated"
	// BehaviorPlanStatusArchived — replaced or abandoned by explicit user choice.
	BehaviorPlanStatusArchived = "archived"
)

// MaxActiveBehaviorPlans caps how many behaviors a user works on simultaneously. More than
// this and every session opens with an interrogation instead of coaching (protocol decision:
// focus beats volume).
const MaxActiveBehaviorPlans = 2

// BehaviorPlan is the cross-session record of one behavior-change commitment, following
// Filipa's Behaviour Change Protocol: identity-based (the behavior is a means; the identity
// is the point), friction-reduction, no guilt. It projects into a recurring Task (TaskID)
// which carries the day-to-day execution and completion; the plan itself carries the "why"
// and the adjustment history. WinsCount only ever goes up — misses are information for the
// coach, never a broken streak for the user.
type BehaviorPlan struct {
	ID     string `gorm:"primaryKey;type:text" json:"id"`
	UserID string `gorm:"index;type:text;not null" json:"userId"`
	Status string `gorm:"type:text;not null;default:active" json:"status"`

	// Behavior is the smallest sustainable version of the behavior ("walk 10 minutes").
	Behavior string `gorm:"type:text;not null" json:"behavior"`
	// Identity is the person this behavior is a vote for ("someone who protects their
	// health") — co-created with the user, in the user's language.
	Identity string `gorm:"type:text" json:"identity"`
	// Motive captures why this matters to the user and the values it serves.
	Motive string `gorm:"type:text" json:"motive"`
	// Trigger is the existing anchor the behavior attaches to ("after my morning coffee").
	Trigger string `gorm:"type:text" json:"trigger"`
	// Context answers when/where/with whom/how they will remember.
	Context string `gorm:"type:text" json:"context"`
	// Frequency in the user's words ("every morning", "3x per week").
	Frequency string `gorm:"type:text" json:"frequency"`
	// Days are the ISO weekdays (1=Mon..7=Sun) projected into the linked recurring Commitment.
	Days      IntSlice `gorm:"type:jsonb" json:"days,omitempty"`
	Obstacles string   `gorm:"type:text" json:"obstacles"`
	PlanB     string   `gorm:"type:text" json:"planB"`

	// Area optionally names the Wheel of Life area this behavior serves — the meaning
	// chain wheel area → behavior → commitment.
	Area *string `gorm:"type:text" json:"area,omitempty"`
	// TaskID links the recurring Commitment that carries daily execution (origin=behavior).
	TaskID *string `gorm:"index;type:text" json:"taskId,omitempty"`

	StartDate *string `gorm:"type:text" json:"startDate,omitempty"`
	// WinsCount counts kept check-ins. It only increments (Lally 2010: a missed day does
	// not undo habit formation, so the data model never presents one as damage).
	WinsCount     int        `gorm:"not null;default:0" json:"winsCount"`
	LastCheckInAt *time.Time `gorm:"type:timestamp with time zone" json:"lastCheckInAt,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;type:timestamp with time zone" json:"updatedAt"`
}

func (p *BehaviorPlan) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return
}

// Behavior check-in statuses.
const (
	BehaviorCheckInKept    = "kept"
	BehaviorCheckInPartial = "partial"
	BehaviorCheckInMissed  = "missed"
)

// BehaviorCheckIn is one follow-up datapoint on a plan: kept / partial / missed plus what
// the user said about it. Misses are stored for the coach (to adjust the plan) and are
// never surfaced to the user as failure.
type BehaviorCheckIn struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	PlanID    string    `gorm:"index;type:text;not null" json:"planId"`
	UserID    string    `gorm:"index;type:text;not null" json:"userId"`
	Status    string    `gorm:"type:text;not null" json:"status"`
	Note      string    `gorm:"type:text" json:"note"`
	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone" json:"createdAt"`
}

func (c *BehaviorCheckIn) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return
}
