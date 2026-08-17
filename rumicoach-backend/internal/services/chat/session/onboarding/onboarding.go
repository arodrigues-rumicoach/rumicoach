// Package onboarding owns the declarative definition of the first-time Onboarding
// session: the greeting, the privacy explanation and the roadmap. It is deliberately
// short — everything from the ideal-life visualization onwards now lives in the
// vision package, which this session hands over to. It depends only on the shared
// models package — never on the chat package — so the chat runtime can consume it
// without creating an import cycle. The imperative plumbing (DB writes, WebSocket
// I/O, ChatSession state) stays in the chat package and calls into this one.
package onboarding

import (
	"fmt"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

// Onboarding is the first-time onboarding session — the intro alone. It implements
// session.Session.
type Onboarding struct{}

// New returns the onboarding session implementation.
func New() *Onboarding { return &Onboarding{} }

func (Onboarding) Name() string                      { return "onboarding" }
func (Onboarding) Type() api.SessionType             { return api.SessionTypeOnboarding }
func (Onboarding) InitialState() models.SessionState { return models.StateOnboardingIntro }

// SystemPersona uses the shared first-session voice, which the Vision session also
// uses: the two are one continuous first meeting from the user's side.
func (Onboarding) SystemPersona(v session.Voice) string {
	return session.FirstSessionPersona(v)
}

func (Onboarding) Instructions(state models.SessionState, ctx session.Context) string {
	instr, _ := Instructions(state, ctx.FirstName)
	return instr
}

func (Onboarding) ToolNames(state models.SessionState) []string { return ToolNames(state) }

func (Onboarding) NextOnCompleteTask(state models.SessionState) (session.Transition, bool) {
	t, ok := NextOnCompleteTask(state)
	return session.Transition{Next: t.Next, Blocked: t.Blocked}, ok
}

func (Onboarding) NeedsRestart(from, to models.SessionState) bool { return NeedsRestart(from, to) }

func (Onboarding) ReviewPrompt() string { return reviewPrompt }

// Instructions returns the task prompt for an onboarding state and true when the
// state belongs to the onboarding flow. firstName personalises the scripted lines.
func Instructions(state models.SessionState, firstName string) (string, bool) {
	switch state {
	case models.StateOnboardingIntro, models.StateLegacyOnboarding:
		return fmt.Sprintf(introInstructions, firstName), true
	default:
		return "", false
	}
}

// ToolNames returns the names of the task tools available in the given onboarding
// state. save_profile_details completes the registration the intro collects (country,
// date of birth, gender). The intro deliberately excludes show_screen and
// save_session_insight: the screens of its mini tour (memories, growth) are driven by the
// silent ◆▣ / ◆▥ markers embedded in the scripts, and there is no insight to capture yet.
//
// start_planned_session and terminate_session are the two exits: the intro closes by
// asking whether to continue straight into the Vision session, so the model needs a
// tool for each answer. Never declare one without the other — a scripted branch whose
// tool is undeclared yields Gemini's "Invalid function call" and a dead session.
func ToolNames(models.SessionState) []string {
	return []string{"complete_current_task", "save_profile_details", "start_planned_session", "terminate_session"}
}

// CompleteTransition describes how complete_current_task advances an onboarding state.
type CompleteTransition struct {
	Next models.SessionState
	// Blocked, when non-empty, is the error message returned because this phase must
	// be advanced by a dedicated tool rather than complete_current_task.
	Blocked string
}

// NextOnCompleteTask returns the transition for an onboarding state and true when
// the state belongs to the onboarding flow (so the chat layer can fall back to the
// generic state machine for other session types).
//
// The intro advances into the Vision session's first state whether or not the user
// continues right now: the state means "the intro is done, Vision is what comes next",
// so a user who declines and hangs up is still routed to Vision (and still exempt from
// balance debits) when they come back.
func NextOnCompleteTask(current models.SessionState) (CompleteTransition, bool) {
	switch current {
	case models.StateOnboardingIntro, models.StateLegacyOnboarding:
		return CompleteTransition{Next: models.StateVisionIdealLife}, true
	default:
		return CompleteTransition{}, false
	}
}

// NeedsRestart reports whether transitioning between the two states must hard-restart
// the Gemini connection. The intro is a single state, and the handover into Vision is
// driven by start_planned_session (which restarts on its own), so nothing here does.
func NeedsRestart(_, _ models.SessionState) bool { return false }
