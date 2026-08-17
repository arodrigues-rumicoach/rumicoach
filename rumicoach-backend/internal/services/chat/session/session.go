// Package session defines the contract every coaching session type implements, plus the
// small value types passed across that boundary. It depends only on the shared models and
// api packages — never on the chat runtime or any concrete session package — so concrete
// sessions (onboarding, movement, values, ...) and the chat engine can both import it
// without creating an import cycle.
package session

import (
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
)

// Voice carries the coach voice attributes the chat layer derives from the user record and
// passes into a session's persona block.
type Voice struct {
	CoachGender string // "male" | "female"
	MentorDesc  string // "fatherly" | "motherly"
	ToneDesc    string // the spoken-tone line
}

// WheelItem is the minimal Wheel-of-Life datum a session may consult when building prompts.
type WheelItem struct {
	Name         string
	CurrentScore float64
	Reasoning    string
}

// Context bundles the per-turn inputs a session needs to build its instructions. The chat
// layer populates it (resolving the first name, loading wheel data, etc.). UserID is provided
// for sessions that compute their own dynamic context (e.g. the check-in's frequency analysis).
type Context struct {
	UserID    string
	FirstName string
	Wheel     []WheelItem
	// PlannedSession, when set, is the deep/weekly/monthly session proposed for the user today
	// that has not been done yet. The daily check-in uses it to offer that session before
	// running the regular check-in. Empty when nothing is planned (or this is not a check-in).
	PlannedSession api.SessionType
}

// Transition is the result of advancing a state via complete_current_task.
type Transition struct {
	Next models.SessionState
	// Blocked, when non-empty, is the JSON error returned because this phase must be
	// advanced by a dedicated tool rather than complete_current_task.
	Blocked string
}

// Session is the behaviour each coaching session type provides. The chat runtime owns the
// WebSocket/Gemini plumbing and the shared (common) tools, and dispatches to the matching
// Session for everything that varies per session type.
type Session interface {
	// Name is the single-word identifier (e.g. "onboarding").
	Name() string
	// Type is the API session type this session handles.
	Type() api.SessionType
	// InitialState is where a fresh session of this type starts.
	InitialState() models.SessionState

	// SystemPersona returns the Identity & Persona / Interaction Style block (Sections 3-4
	// of the system prompt) for this session.
	SystemPersona(v Voice) string
	// Instructions returns the active-task prompt (Section 9) for the given state.
	Instructions(state models.SessionState, ctx Context) string
	// ToolNames returns the names of the task tools available in the given state. The chat
	// layer adds the common tools and resolves names to full definitions.
	ToolNames(state models.SessionState) []string

	// NextOnCompleteTask returns the transition for complete_current_task, and false when
	// the state is not part of this session's flow.
	NextOnCompleteTask(state models.SessionState) (Transition, bool)
	// NeedsRestart reports whether transitioning between two states requires a Gemini
	// connection restart.
	NeedsRestart(from, to models.SessionState) bool

	// ReviewPrompt returns the QA/review instruction used to grade this session's transcripts.
	ReviewPrompt() string
}

// DisplayName maps a session type to a warm, human-friendly name a coach can speak aloud.
func DisplayName(t api.SessionType) string {
	switch t {
	case api.SessionTypeSessionVision:
		return "Ideal Life Vision"
	case api.SessionTypeSessionMovement:
		return "Obstacles and Movement"
	case api.SessionTypeSessionValues:
		return "Values"
	case api.SessionTypeSessionEnergy:
		return "Energy and Vitality"
	case api.SessionTypeSessionDecisions:
		return "Fears and Decisions"
	case api.SessionTypeSessionBeliefs:
		return "Beliefs"
	case api.SessionTypeSessionIdentity:
		return "Identity"
	case api.SessionTypeSessionAcceptance:
		return "Expectations and Acceptance"
	case api.SessionTypeSessionPriorities:
		return "Priorities"
	default:
		return "session"
	}
}

// Registry maps API session types to their implementations.
type Registry struct {
	byType map[api.SessionType]Session
}

// NewRegistry builds a Registry from the given sessions, keyed by their Type().
func NewRegistry(sessions ...Session) *Registry {
	r := &Registry{byType: make(map[api.SessionType]Session, len(sessions))}
	for _, s := range sessions {
		r.byType[s.Type()] = s
	}
	return r
}

// Get returns the session for the given type and whether one is registered.
func (r *Registry) Get(t api.SessionType) (Session, bool) {
	s, ok := r.byType[t]
	return s, ok
}
