package companion

import (
	"testing"

	"github.com/rumi/rumi-be/internal/services/aiusage"
)

// Every model this package calls must have a price, or its spend lands in the cost
// ledger with a NULL cost and quietly reads as free.
//
// The guard lives here rather than in the aiusage package because only this package
// can see what the defaults actually are: a test over there would have to hard-code
// them, and would then keep passing after someone changed one. Whoever changes a
// default model gets this failure in the same package they edited.
func TestCompanionModelsArePriced(t *testing.T) {
	// Cleared so the compiled-in defaults are what gets checked, not whatever the
	// developer's shell happens to export.
	for _, key := range []string{"GEMINI_COMPANION_MODEL", "GEMINI_TRANSCRIBE_MODEL", "GEMINI_TTS_MODEL"} {
		t.Setenv(key, "")
	}

	for _, c := range []struct {
		what  string
		model string
	}{
		{"companion chat", chatModel()},
		{"voice-note transcription", transcribeModel()},
		{"voice-note synthesis", ttsModel()},
	} {
		if _, ok := aiusage.PriceFor(c.model); !ok {
			t.Errorf("%s uses %q, which has no entry in aiusage/prices.go — its cost would record as NULL",
				c.what, c.model)
		}
	}
}
