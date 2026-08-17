// Package vision owns the declarative definition of the Vision session: the ideal-life
// visualization, the Wheel of Life, the focus-area commitment and the closing. It was
// the tail of the onboarding session until onboarding was reduced to its intro; the
// scripts, tools and transitions moved here unchanged, so what the user hears is the
// same. Like the other session packages it depends only on models/api and the shared
// session contract — never on the chat runtime — so the runtime can consume it without
// an import cycle.
package vision

import (
	"fmt"
	"strings"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

// WheelItem is the minimal Wheel-of-Life datum the prompts need, re-exported from the
// session package so existing call sites keep working.
type WheelItem = session.WheelItem

// Vision is the session that follows the onboarding intro. It implements session.Session.
type Vision struct{}

// New returns the vision session implementation.
func New() *Vision { return &Vision{} }

func (Vision) Name() string                      { return "vision" }
func (Vision) Type() api.SessionType             { return api.SessionTypeSessionVision }
func (Vision) InitialState() models.SessionState { return models.StateVisionIdealLife }

// SystemPersona uses the shared first-session voice: the Vision session continues the
// conversation the intro opened, so it must not sound like a different coach.
func (Vision) SystemPersona(v session.Voice) string {
	return session.FirstSessionPersona(v)
}

func (Vision) Instructions(state models.SessionState, ctx session.Context) string {
	instr, _ := Instructions(state, ctx.FirstName, ctx.Wheel)
	return instr
}

func (Vision) ToolNames(state models.SessionState) []string { return ToolNames(state) }

func (Vision) NextOnCompleteTask(state models.SessionState) (session.Transition, bool) {
	t, ok := NextOnCompleteTask(state)
	return session.Transition{Next: t.Next, Blocked: t.Blocked}, ok
}

func (Vision) NeedsRestart(from, to models.SessionState) bool { return NeedsRestart(from, to) }

func (Vision) ReviewPrompt() string { return reviewPrompt }

// Instructions returns the task prompt for a Vision state and true when the state
// belongs to the Vision flow. firstName personalises the scripted lines; wheel is only
// consulted for the Wheel-of-Life state.
func Instructions(state models.SessionState, firstName string, wheel []WheelItem) (string, bool) {
	switch state {
	case models.StateVisionIdealLife:
		// Two placeholders: the welcome in PHASE 0 and the visualization script in PHASE 1.
		return fmt.Sprintf(idealLifeVisionInstructions, firstName, firstName), true
	case models.StateVisionWheelOfLife:
		return wheelInstructions(wheel), true
	case models.StateVisionMetaphor:
		return metaphorInstructions, true
	case models.StateVisionEmotionalClosing:
		return emotionalClosingInstructions, true
	case models.StateVisionEndingSession:
		return endingSessionInstructions, true
	default:
		return "", false
	}
}

// WheelDynamicTransition returns the prompt injected after an area is saved (or the areas
// are first created), telling the model which area to ask for next, or to move to the
// completion step once all areas are scored. Only call this when the state is the wheel.
// introSkipped marks the setup turn where the model called set_wheel_of_life_categories
// WITHOUT speaking the introduction script first (observed in QA: the user is then asked
// for a score with no framing at all) — the first-area cue then has the model recover the
// introduction before asking.
func WheelDynamicTransition(wheel []WheelItem, introSkipped bool) string {
	nextPending := firstPendingCategory(wheel)
	if nextPending == "" {
		return wheelCompletionTransition
	}
	if !anyScored(wheel) {
		// First area, right after the areas were created — there is no previous area to reference.
		if introSkipped {
			return fmt.Sprintf(wheelFirstCategoryMissingIntroTransition, nextPending)
		}
		return fmt.Sprintf(wheelFirstCategoryTransition, nextPending, nextPending, nextPending)
	}
	return fmt.Sprintf(wheelNextCategoryTransition, nextPending, nextPending)
}

// WheelReconnectPrompt returns the connection-start trigger for a client that
// reconnected while the Wheel of Life assessment was already underway (the areas
// exist). No dialogue summary exists on client reconnects, so the wheel template's
// own "IF RESUMING" clause never applies — and the generic "deliver the first step"
// trigger makes the model re-deliver the whole introduction (and re-create the
// areas) on every app restart. This cue resumes at the right point instead.
func WheelReconnectPrompt(wheel []WheelItem) string {
	nextPending := firstPendingCategory(wheel)
	if nextPending == "" {
		return wheelReconnectCompletionPrompt
	}
	return fmt.Sprintf(wheelReconnectPrompt, nextPending)
}

func anyScored(wheel []WheelItem) bool {
	for _, item := range wheel {
		if item.CurrentScore > 0 {
			return true
		}
	}
	return false
}

// ToolNames returns the names of the task tools available in the given Vision state.
// Because the Gemini connection only restarts when entering/leaving the Wheel of Life,
// each phase exposes every tool reachable across its in-session transitions. The chat
// layer resolves these names to full tool definitions.
func ToolNames(state models.SessionState) []string {
	// The opening stretch (ideal-life vision → wheel) deliberately excludes show_screen and
	// save_session_insight: screens there are driven exclusively through the silent ◆ markers
	// embedded in the scripts (a redundant show_screen call opens the screen at the wrong
	// moment), and the session insight is only captured in the closing stretch. The tool list
	// refreshes at the wheel restart points, so the post-wheel phases regain both.
	switch state {
	case models.StateVisionWheelOfLife:
		return []string{"complete_current_task", "set_wheel_of_life_categories", "update_wheel_of_life"}
	case models.StateVisionMetaphor, models.StateVisionEmotionalClosing, models.StateVisionEndingSession:
		// save_commitments/update_commitment_plan are deliberately NOT declared: no script in this
		// session uses them (commitment planning belongs to the Movement session), and having
		// them available let the model improvise an off-script commitment-plan detour when a
		// closing transition stalled (QA). Declared tools must always have a scripted
		// trigger — and vice versa. save_vision_commitment is different: it is narrowly scoped
		// to a single free-text field with no type/date/days for the model to improvise a
		// schedule with, and has exactly one scripted trigger (Step 4 of emotionalClosingInstructions).
		return []string{"complete_current_task", "save_focus", "terminate_session", "show_screen", "save_session_insight", "save_vision_commitment"}
	default: // VISION_IDEAL_LIFE
		return []string{"complete_current_task", "save_ideal_life_vision"}
	}
}

// CompleteTransition describes how complete_current_task advances a Vision state.
type CompleteTransition struct {
	Next models.SessionState
	// Blocked, when non-empty, is the error message returned because this phase must
	// be advanced by a dedicated tool (e.g. save_ideal_life_vision) rather than
	// complete_current_task.
	Blocked string
}

// NextOnCompleteTask returns the transition for a Vision state and true when the state
// belongs to the Vision flow (so the chat layer can fall back to the generic state
// machine for other session types).
func NextOnCompleteTask(current models.SessionState) (CompleteTransition, bool) {
	switch current {
	case models.StateVisionIdealLife:
		return CompleteTransition{Blocked: `{"status": "error", "message": "CRITICAL: You are NOT allowed to use 'complete_current_task' to transition out of the Ideal Life Vision phase. You MUST call 'save_ideal_life_vision' to save the vision, which will automatically advance the session."}`}, true
	case models.StateVisionWheelOfLife:
		return CompleteTransition{Next: models.StateVisionMetaphor}, true
	case models.StateVisionMetaphor:
		return CompleteTransition{Blocked: `{"status": "error", "message": "CRITICAL: You are NOT allowed to use 'complete_current_task' to transition out of the METAPHOR phase. Once the user has named their priority area and explained why, call 'save_focus' (silently), which will automatically advance the session."}`}, true
	case models.StateVisionEmotionalClosing:
		return CompleteTransition{Next: models.StateVisionEndingSession}, true
	case models.StateVisionEndingSession:
		// The Vision session finishes into the CHECKIN resting state. The next deep
		// session (movement, etc.) is chosen by the journey endpoint from session history.
		return CompleteTransition{Next: models.StateCheckin}, true
	default:
		return CompleteTransition{}, false
	}
}

// NeedsRestart reports whether transitioning between the two states must hard-restart
// the Gemini connection. We only restart around the Wheel of Life — the most complex
// task — i.e. when entering or leaving it.
func NeedsRestart(from, to models.SessionState) bool {
	return from == models.StateVisionWheelOfLife || to == models.StateVisionWheelOfLife
}

// --- internal helpers ---

func firstPendingCategory(wheel []WheelItem) string {
	for _, item := range wheel {
		if item.CurrentScore <= 0 {
			return item.Name
		}
	}
	return ""
}

func wheelInstructions(wheel []WheelItem) string {
	var nextPending string
	var status strings.Builder
	for _, item := range wheel {
		if item.CurrentScore > 0 {
			status.WriteString(fmt.Sprintf("- [COMPLETED] %s: Current %.0f/10. Reasoning: %s\n", item.Name, item.CurrentScore, item.Reasoning))
		} else {
			status.WriteString(fmt.Sprintf("- [PENDING] %s\n", item.Name))
			if nextPending == "" {
				nextPending = item.Name
			}
		}
	}

	prompt := fmt.Sprintf(wheelInstructionsTemplate, status.String())

	var dynamic []string
	if len(wheel) > 0 && nextPending == "" {
		// All areas scored — guide the completion/confirmation step. While areas are still
		// pending, the template's per-area protocol (plus the per-save "ask next area" prompt)
		// drives the flow, so no dynamic prefix is needed.
		dynamic = append(dynamic, wheelCompletionInstruction)
	}

	if len(dynamic) > 0 {
		prompt = strings.Replace(prompt, "[TASK INSTRUCTION]", "[TASK INSTRUCTION]\n"+strings.Join(dynamic, "\n\n"), 1)
	}
	return prompt
}
