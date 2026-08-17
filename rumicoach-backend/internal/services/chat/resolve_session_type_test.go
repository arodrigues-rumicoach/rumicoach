package chat

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// At the CHECKIN resting state the client-supplied session type is only a suggestion. QA:
// the app opened session_type=session_vision at CHECKIN — the session fell back to the
// check-in prompt but stayed TYPED session_vision, which skipped the planned-session
// lookup (Movement was due and never offered), would have logged a long chat as a
// completed Vision deep session (pushing the journey gates forward), and keyed the
// summary builder into a Vision synthesis panel for a mere check-in.
func TestResolveSessionTypeAtCheckin(t *testing.T) {
	cases := map[api.SessionType]api.SessionType{
		// The opening pair is done once and never revisited — stale requests resolve away.
		api.SessionTypeSessionVision: api.SessionTypeCheckin,
		api.SessionTypeOnboarding:    api.SessionTypeCheckin,
		"":                           api.SessionTypeCheckin,
		api.SessionTypeCheckin:       api.SessionTypeCheckin,
		// A later deep session launched directly by the app is honored.
		api.SessionTypeSessionMovement: api.SessionTypeSessionMovement,
		api.SessionTypeSessionValues:   api.SessionTypeSessionValues,
		// Garbage falls back to the check-in rather than a dead session.
		"not_a_real_session": api.SessionTypeCheckin,
	}
	for requested, want := range cases {
		s := &ChatSession{logger: zap.NewNop(), SessionType: requested, User: checkinUser()}
		s.resolveSessionType()
		if s.SessionType != want {
			t.Errorf("requested %q at CHECKIN resolved to %q, want %q", requested, s.SessionType, want)
		}
	}
}

// checkinUser is parked at CHECKIN with the opening pair genuinely behind them: the
// profile details collected and the ideal-life vision written. CHECKIN alone does not
// prove that (see the test below), and resolveSessionType now checks.
func checkinUser() *models.User {
	state := string(models.StateCheckin)
	dob := time.Date(1990, 5, 3, 0, 0, 0, 0, time.UTC)
	gender, country := "male", "PT"
	visionAt := time.Now().Add(-30 * 24 * time.Hour)
	return &models.User{
		State: &state, DateOfBirth: &dob, Gender: &gender, Country: &country,
		IdealLifeVisionSetAt: &visionAt,
	}
}

// CHECKIN does not mean the opening pair finished. terminate_session parks a Vision
// session the user cut short at CHECKIN, leaving the ideal-life vision unwritten — and
// the journey proposes Vision to exactly those users. Resolving them to a check-in made
// the app offer one session and the server run another, and (because the free allowance
// follows the artifacts) run it for nothing. Whatever the client asks for, they get the
// session they still owe.
func TestResolveSessionTypeAtCheckinWithUnfinishedOpeningPair(t *testing.T) {
	requested := []api.SessionType{
		"", api.SessionTypeCheckin, api.SessionTypeSessionMovement,
		api.SessionTypeSessionValues, api.SessionTypeSessionVision, "not_a_real_session",
	}

	t.Run("vision never written", func(t *testing.T) {
		for _, req := range requested {
			user := checkinUser()
			user.IdealLifeVisionSetAt = nil
			s := &ChatSession{logger: zap.NewNop(), SessionType: req, User: user}
			s.resolveSessionType()
			if s.SessionType != api.SessionTypeSessionVision {
				t.Errorf("requested %q with no vision: resolved to %q, want session_vision", req, s.SessionType)
			}
		}
	})

	t.Run("profile details never collected", func(t *testing.T) {
		for _, req := range requested {
			user := checkinUser()
			user.Country = nil
			s := &ChatSession{logger: zap.NewNop(), SessionType: req, User: user}
			s.resolveSessionType()
			if s.SessionType != api.SessionTypeSessionVision {
				t.Errorf("requested %q with no country: resolved to %q, want session_vision", req, s.SessionType)
			}
		}
	})
}
