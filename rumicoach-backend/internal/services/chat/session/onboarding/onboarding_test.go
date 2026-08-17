package onboarding

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/internal/models"
)

func TestInstructionsCoverOnboardingStates(t *testing.T) {
	intro, ok := Instructions(models.StateOnboardingIntro, "Armando")
	if !ok {
		t.Fatal("Instructions(ONBOARDING_INTRO) returned ok=false")
	}
	if !strings.Contains(intro, "[TASK INSTRUCTION]") {
		t.Error("intro missing task marker")
	}
	if !strings.Contains(intro, "Hello, Armando!") {
		t.Errorf("intro not personalised: %q", intro[:60])
	}

	// The legacy 'ONBOARDING' value (the old users.state column default) maps to the
	// intro — falling through here sent brand-new users to the daily check-in prompt.
	legacy, ok := Instructions(models.StateLegacyOnboarding, "Armando")
	if !ok || legacy != intro {
		t.Error("legacy ONBOARDING state should yield the intro instructions")
	}

	// The states that moved to the Vision session are no longer handled here.
	for _, st := range []models.SessionState{
		models.StateVisionIdealLife,
		models.StateVisionWheelOfLife,
		models.StateVisionMetaphor,
		models.StateVisionEmotionalClosing,
		models.StateVisionEndingSession,
		models.StateCheckin,
	} {
		if _, ok := Instructions(st, "Armando"); ok {
			t.Errorf("%s should not be handled by the onboarding package", st)
		}
	}
}

// The intro closes by asking whether to continue into Vision, and the model needs a tool
// for each answer. A declared-but-untriggered tool (or the reverse) is how these sessions
// die: Gemini rejects a call to an undeclared tool and the session goes silent.
func TestIntroDeclaresBothExits(t *testing.T) {
	tools := ToolNames(models.StateOnboardingIntro)
	joined := strings.Join(tools, ",")
	for _, want := range []string{"start_planned_session", "terminate_session"} {
		if !strings.Contains(joined, want) {
			t.Errorf("intro must declare %s, got: %s", want, joined)
		}
	}

	intro, _ := Instructions(models.StateOnboardingIntro, "Armando")
	for _, want := range []string{"start_planned_session", "terminate_session"} {
		if !strings.Contains(intro, want) {
			t.Errorf("intro script must trigger %s", want)
		}
	}
	// The wheel/vision tools belong to the Vision session; declaring them here would let
	// the model improvise its way into an exercise this session does not run.
	for _, unwanted := range []string{"save_ideal_life_vision", "set_wheel_of_life_categories", "save_focus"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("intro must not declare %s, got: %s", unwanted, joined)
		}
	}
}

func TestTransitionsAndRestart(t *testing.T) {
	// The intro advances into the Vision session's first state, so a user who declines
	// to continue right now is still routed to Vision when they come back.
	if tr, ok := NextOnCompleteTask(models.StateOnboardingIntro); !ok || tr.Next != models.StateVisionIdealLife {
		t.Errorf("intro should advance to the vision phase, got %+v ok=%v", tr, ok)
	}
	if tr, ok := NextOnCompleteTask(models.StateLegacyOnboarding); !ok || tr.Next != models.StateVisionIdealLife {
		t.Errorf("legacy onboarding should advance to the vision phase, got %+v ok=%v", tr, ok)
	}
	// States this session no longer owns.
	for _, st := range []models.SessionState{models.StateVisionWheelOfLife, models.StateCheckin} {
		if _, ok := NextOnCompleteTask(st); ok {
			t.Errorf("%s transition should not be owned by onboarding", st)
		}
	}

	// The intro is a single state and the handover restarts on its own.
	if NeedsRestart(models.StateOnboardingIntro, models.StateVisionIdealLife) {
		t.Error("intro→vision should not need a restart from this session")
	}
}

// The tab is labelled "Journey" in the app; the screen identifier the backend passes is
// still "growth". Scripts are spoken aloud, so what they name has to be what the user
// reads on their screen — a coach saying "your Journey screen" while the tab says Journey
// makes the product feel like two products.
//
// The identifier is deliberately NOT renamed: the app switches on screen === 'growth', so
// changing the wire value would silently stop the screen opening at all. That is exactly
// why this needs pinning — the two are easy to conflate, and only one of them is safe to
// change.
func TestTourNamesTheScreenTheUserSees(t *testing.T) {
	prompt, ok := Instructions(models.StateOnboardingIntro, "Armando")
	if !ok {
		t.Fatal("no instructions for the intro state")
	}

	if strings.Contains(strings.ToLower(prompt), "Journey screen") {
		t.Errorf("the tour still calls it the Journey screen; the app calls it Journey:\n%s", prompt)
	}
	if !strings.Contains(prompt, "◆▥ And this is your Journey") {
		t.Error("the spoken script should introduce the screen by the name the user reads")
	}
	// And the marker that opens it must survive intact — it is parsed byte-for-byte.
	if !strings.Contains(prompt, "◆▥") {
		t.Error("the Journey screen marker is missing from the tour")
	}
}
