package chat

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// The ideal-life exploration mandates follow-up questions, but the model saved the vision
// right after the user's first answer, skipping the deepening entirely (QA: "não me
// perguntou nem aquilo que eu iria sentir, nem com quem é que eu estaria"). A save that
// arrives before at least one follow-up was answered is rejected exactly once.
func TestVisionSaveRejectedBeforeFollowUp(t *testing.T) {
	state := string(models.StateVisionIdealLife)
	s := &ChatSession{
		logger:      zap.NewNop(),
		SessionType: api.SessionTypeSessionVision,
		User:        &models.User{ID: "user-1", State: &state},
	}
	s.visionUserTurns = 1 // only the opening answer

	out, err := s.handleSaveIdealLifeVision(map[string]interface{}{"vision": "Uma vida com mais tempo."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "REJECTED") || !strings.Contains(out, "follow-up") {
		t.Fatalf("a save before any follow-up must be rejected with follow-up guidance, got: %s", out)
	}
	if s.User.IdealLifeVision != nil {
		t.Error("the vision must not be stored on rejection")
	}
	if !s.visionEarlySaveRejected {
		t.Error("rejection must be recorded so the retry always passes (resumed sessions)")
	}

	// With the exploration done (two or more user turns), the guard lets the call through
	// to the DB write — which fails on the missing test DB, proving the guard itself no
	// longer blocks.
	s2 := &ChatSession{
		logger:      zap.NewNop(),
		SessionType: api.SessionTypeSessionVision,
		User:        &models.User{ID: "user-1", State: &state},
	}
	s2.visionUserTurns = 3
	if out2, _ := s2.handleSaveIdealLifeVision(map[string]interface{}{"vision": "Uma vida com mais tempo."}); strings.Contains(out2, "REJECTED: the exploration") {
		t.Fatalf("an explored vision must not be rejected, got %q", out2)
	}
}

// Intro turns must not count as exploration: during the onboarding intro the stored state
// row is already VISION_IDEAL_LIFE, but CurrentState maps it to the intro — only turns
// spoken inside the actual Vision session count toward the follow-up guard.
func TestVisionTurnCounterIgnoresIntro(t *testing.T) {
	state := string(models.StateVisionIdealLife)
	s := &ChatSession{
		logger:      zap.NewNop(),
		SessionType: api.SessionTypeOnboarding,
		User:        &models.User{ID: "user-1", State: &state},
	}
	if s.CurrentState() == models.StateVisionIdealLife {
		t.Fatal("intro turns would be miscounted: CurrentState must map the intro away from VISION_IDEAL_LIFE")
	}
}
