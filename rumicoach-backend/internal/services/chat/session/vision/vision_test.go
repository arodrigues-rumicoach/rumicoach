package vision

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/internal/models"
)

func TestInstructionsCoverAllVisionStates(t *testing.T) {
	states := []models.SessionState{
		models.StateVisionIdealLife,
		models.StateVisionWheelOfLife,
		models.StateVisionMetaphor,
		models.StateVisionEmotionalClosing,
		models.StateVisionEndingSession,
	}
	for _, st := range states {
		got, ok := Instructions(st, "Armando", nil)
		if !ok {
			t.Errorf("Instructions(%s) returned ok=false", st)
		}
		if !strings.Contains(got, "[TASK INSTRUCTION]") {
			t.Errorf("Instructions(%s) missing task marker", st)
		}
	}

	// The visualization script personalises the invitation. Both placeholders must be
	// filled: a missing Sprintf argument silently renders "%!s(MISSING)" into a script
	// the coach then reads aloud.
	idealLife, _ := Instructions(models.StateVisionIdealLife, "Armando", nil)
	if strings.Contains(idealLife, "%!s(MISSING)") || strings.Contains(idealLife, "%!(EXTRA") {
		t.Error("ideal life prompt has a Sprintf argument mismatch")
	}
	if n := strings.Count(idealLife, "Armando"); n < 2 {
		t.Errorf("first name should fill both the welcome and the visualization script, got %d occurrences", n)
	}

	// The session opens on its own now that it is no longer the tail of onboarding:
	// a welcome, and a short overview of what the session covers.
	for _, want := range []string{"PHASE 0", "Hello again", "three things"} {
		if !strings.Contains(idealLife, want) {
			t.Errorf("ideal life prompt should open with a welcome and overview, missing %q", want)
		}
	}
	// Arriving straight from the intro must not replay the welcome, and the overview
	// must not stop for an answer that the script never asks for.
	for _, want := range []string{"do NOT greet again", "DO NOT STOP AND WAIT after this"} {
		if !strings.Contains(idealLife, want) {
			t.Errorf("ideal life prompt missing guard: %q", want)
		}
	}

	// States outside this session are not handled here — including the onboarding
	// intro, which belongs to the onboarding package now.
	for _, st := range []models.SessionState{models.StateCheckin, models.StateOnboardingIntro} {
		if _, ok := Instructions(st, "Armando", nil); ok {
			t.Errorf("%s should not be handled by the vision package", st)
		}
	}
}

func TestWheelDynamicTransitionIntroRecovery(t *testing.T) {
	fresh := []WheelItem{{Name: "Saúde"}, {Name: "Relações"}}

	// Normal setup turn (intro was spoken): the standard first-area cue, which forbids
	// re-delivering the introduction.
	got := WheelDynamicTransition(fresh, false)
	if !strings.Contains(got, "'Saúde'") || !strings.Contains(got, "Do NOT re-deliver the introduction") {
		t.Errorf("expected standard first-area cue, got: %s", got)
	}

	// Silent setup turn (model called the tool without speaking): the cue must instead
	// have the model recover the introduction before asking.
	got = WheelDynamicTransition(fresh, true)
	if !strings.Contains(got, "did NOT speak the introduction") || !strings.Contains(got, "'Saúde'") {
		t.Errorf("expected intro-recovery cue, got: %s", got)
	}

	// Once any area is scored, introSkipped is irrelevant — next-area cue as usual.
	scored := []WheelItem{{Name: "Saúde", CurrentScore: 8}, {Name: "Relações"}}
	got = WheelDynamicTransition(scored, true)
	if !strings.Contains(got, "The previous area was just saved") {
		t.Errorf("expected next-area cue, got: %s", got)
	}
}

func TestWheelReconnectPrompt(t *testing.T) {
	// Mid-assessment reconnect resumes at the first pending area and forbids replaying setup.
	wheel := []WheelItem{
		{Name: "Saúde", CurrentScore: 4, Reasoning: "ok"},
		{Name: "Relações", CurrentScore: 0},
		{Name: "Dinheiro", CurrentScore: 0},
	}
	got := WheelReconnectPrompt(wheel)
	if !strings.Contains(got, "'Relações'") {
		t.Errorf("reconnect prompt should target the first pending area, got: %s", got)
	}
	if !strings.Contains(got, "FORBIDDEN from re-delivering the Wheel of Life introduction") {
		t.Error("reconnect prompt must forbid re-delivering the introduction")
	}

	// All areas scored: resume at the final confirmation question instead.
	for i := range wheel {
		wheel[i].CurrentScore = 5
	}
	got = WheelReconnectPrompt(wheel)
	if !strings.Contains(got, "final confirmation question") {
		t.Errorf("all-scored reconnect should cue the completion step, got: %s", got)
	}
}

func TestWheelInstructionsAssembly(t *testing.T) {
	wheel := []WheelItem{
		{Name: "Health", CurrentScore: 7, Reasoning: "good"},
		{Name: "Money", CurrentScore: 0},
	}
	got, ok := Instructions(models.StateVisionWheelOfLife, "Armando", wheel)
	if !ok {
		t.Fatal("wheel state not handled")
	}
	if !strings.Contains(got, "- [COMPLETED] Health: Current 7/10. Reasoning: good") {
		t.Error("missing completed Health line")
	}
	if !strings.Contains(got, "- [PENDING] Money") {
		t.Error("missing pending Money line")
	}
	// While an area is still pending, no completion prefix is prepended — the per-area
	// protocol plus the per-save "ask next area" prompt drive the flow.
	if strings.Contains(got, "ACTION REQUIRED (PHASE 3: COMPLETION)") {
		t.Error("completion prefix should not appear while an area is pending")
	}
}

func TestTransitionsAndTools(t *testing.T) {
	// The ideal-life vision cannot be completed via complete_current_task — only
	// save_ideal_life_vision advances it.
	if tr, ok := NextOnCompleteTask(models.StateVisionIdealLife); !ok || tr.Blocked == "" {
		t.Errorf("ideal life vision should be blocked, got %+v ok=%v", tr, ok)
	}
	// The wheel advances into the metaphor phase.
	if tr, ok := NextOnCompleteTask(models.StateVisionWheelOfLife); !ok || tr.Next != models.StateVisionMetaphor {
		t.Errorf("wheel should advance to metaphor, got %+v ok=%v", tr, ok)
	}
	// Vision ends into the CHECKIN resting state.
	if tr, _ := NextOnCompleteTask(models.StateVisionEndingSession); tr.Next != models.StateCheckin {
		t.Errorf("ending should advance to checkin, got %s", tr.Next)
	}
	// States outside this session are not owned here.
	for _, st := range []models.SessionState{models.StateCheckin, models.StateOnboardingIntro} {
		if _, ok := NextOnCompleteTask(st); ok {
			t.Errorf("%s transition should not be owned by vision", st)
		}
	}

	// Tool scoping: wheel phase exposes the wheel tools, not the vision tool.
	wheelTools := strings.Join(ToolNames(models.StateVisionWheelOfLife), ",")
	if !strings.Contains(wheelTools, "update_wheel_of_life") || strings.Contains(wheelTools, "save_ideal_life_vision") {
		t.Errorf("wheel tool scoping wrong: %s", wheelTools)
	}

	// Restart only around the wheel.
	if !NeedsRestart(models.StateVisionIdealLife, models.StateVisionWheelOfLife) {
		t.Error("entering the wheel should need a restart")
	}
	if NeedsRestart(models.StateVisionMetaphor, models.StateVisionEmotionalClosing) {
		t.Error("metaphor→closing should not need a restart")
	}
}
