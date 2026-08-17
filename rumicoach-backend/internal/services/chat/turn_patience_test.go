package chat

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/services/chat/providers"
)

// The provider default end-of-turn detection is tuned for quick back-and-forth and talks
// over anyone pausing mid-thought — QA watched Rumi cut a user off with the very cue meant
// to give him room ("take your time..."). Zero must stay a true no-op so an unsupported
// field can never break every session.
func TestSetupMessageCarriesTurnDetection(t *testing.T) {
	config.AppConfig = &config.Config{GeminiLiveModel: "gemini-live-test"}
	t.Cleanup(func() { config.AppConfig = nil })

	for name, p := range map[string]providers.GeminiProvider{
		"aistudio": &providers.AIStudioProvider{},
		"vertex":   &providers.VertexProvider{},
	} {
		t.Run(name, func(t *testing.T) {
			withPatience := p.BuildSetupMessage("Kore", "sys", nil, "", "",
				providers.TurnDetection{SilenceDurationMs: 2600})
			setup := withPatience["setup"].(map[string]interface{})
			rti, ok := setup["realtimeInputConfig"].(map[string]interface{})
			if !ok {
				t.Fatal("realtimeInputConfig missing from setup")
			}
			aad := rti["automaticActivityDetection"].(map[string]interface{})
			if aad["silenceDurationMs"] != 2600 {
				t.Errorf("silenceDurationMs = %v, want 2600", aad["silenceDurationMs"])
			}
			if aad["endOfSpeechSensitivity"] != "END_SENSITIVITY_LOW" {
				t.Errorf("endOfSpeechSensitivity = %v, want END_SENSITIVITY_LOW", aad["endOfSpeechSensitivity"])
			}

			bare := p.BuildSetupMessage("Kore", "sys", nil, "", "", providers.TurnDetection{})
			if _, present := bare["setup"].(map[string]interface{})["realtimeInputConfig"]; present {
				t.Error("zero SilenceDurationMs must leave the setup untouched")
			}
		})
	}
}

// The bare-score guard trusts utterance length to tell a score from a reasoning. Both
// strings below are VERBOSE bare scores QA watched slip past the original threshold of 25
// and get saved with fabricated reasoning — once as the priority area itself. This pins the
// threshold above them, and below the shortest genuine reasoning seen in the same logs.
func TestBareScoreThresholdCoversVerboseScores(t *testing.T) {
	for _, verbose := range []string{
		"Okay. Um I guess it's a six.",                   // QA 2026-08-03, saved as Health = 6
		"say like this a seven on eight and maybe eight", // QA 2026-08-02, saved as Relations = 8
	} {
		if utf8.RuneCountInString(verbose) >= bareScoreMinReasoningRunes {
			t.Errorf("verbose bare score %q (%d runes) must stay under the threshold (%d)",
				verbose, utf8.RuneCountInString(verbose), bareScoreMinReasoningRunes)
		}
	}
	genuine := "I have a good family that supports me, wife and kids, so I'm quite happy with that."
	if utf8.RuneCountInString(genuine) < bareScoreMinReasoningRunes {
		t.Errorf("a genuine reasoning (%d runes) must clear the threshold (%d)",
			utf8.RuneCountInString(genuine), bareScoreMinReasoningRunes)
	}
}

// An age can never become a birthday. QA: asked "how old are you?", heard "34", and stored
// a date carrying TODAY'S day and month with a guessed year — day and month the user never
// gave, shown back to them on their profile.
func TestRejectsAgeDerivedBirthDate(t *testing.T) {
	s := newProfileTestSession(t)
	s.Location = time.UTC
	today := time.Now().In(time.UTC)

	ageDerived := time.Date(today.Year()-34, today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).
		Format("2006-01-02")

	out, err := s.handleSaveProfileDetails(map[string]interface{}{"date_of_birth": ageDerived})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "AGE") {
		t.Errorf("expected the reply to explain the age/date confusion, got: %s", out)
	}
	if s.User.DateOfBirth != nil {
		t.Errorf("an age-derived date must not be stored, got %v", s.User.DateOfBirth)
	}

	// Rejected once only: someone genuinely born on this day and month must still get through.
	if _, err := s.handleSaveProfileDetails(map[string]interface{}{"date_of_birth": ageDerived}); err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if s.User.DateOfBirth == nil {
		t.Error("a repeated date must be accepted — the user really was born on this day")
	}

	// A date that is not today's day/month is never touched by the guard.
	s2 := newProfileTestSession(t)
	s2.Location = time.UTC
	if _, err := s2.handleSaveProfileDetails(map[string]interface{}{"date_of_birth": "1990-05-03"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s2.User.DateOfBirth == nil {
		t.Error("an ordinary birth date must be saved")
	}
}
