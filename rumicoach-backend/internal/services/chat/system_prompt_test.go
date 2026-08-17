package chat

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
)

func TestBuildSystemInstructionComposition(t *testing.T) {
	name := "Armando"
	female := "despina" // maps to a female coach voice
	user := &models.User{Name: &name, CoachVoice: &female}

	prompt := BuildSystemInstruction(api.SessionTypeOnboarding, user, nil, nil, nil, nil, nil)

	// All eight shared/per-session section headers must be present and in order.
	markers := []string{
		"### 1. DYNAMIC CONTEXT & PROFILE",
		"### 2. PRIME DIRECTIVE: LANGUAGE & LOCALIZATION",
		"### 3. IDENTITY & PERSONA",
		"### 4. INTERACTION STYLE",
		"### 5. SAFETY PROTOCOL",
		"### 6. AUDIO & TRANSCRIPTION HANDLING",
		"### 7. PREVENTING REPETITION LOOPS",
		"### 8. FUNCTION CALLING & TOOL SYNTAX",
	}
	last := -1
	for _, m := range markers {
		idx := strings.Index(prompt, m)
		if idx == -1 {
			t.Fatalf("section %q missing from composed prompt", m)
		}
		if idx <= last {
			t.Fatalf("section %q out of order", m)
		}
		last = idx
	}

	// User-data-driven header is shared and personalised.
	if !strings.Contains(prompt, "- Name: Armando") {
		t.Error("profile name missing from header")
	}
	// Female coach voice drives the persona's tone/mentor wording.
	if !strings.Contains(prompt, "motherly") || !strings.Contains(prompt, "Female, warm") {
		t.Error("persona did not reflect the female coach voice")
	}
}

func TestSystemPersonaSelectedPerSession(t *testing.T) {
	name := "Armando"
	user := &models.User{Name: &name}

	onboardingPrompt := BuildSystemInstruction(api.SessionTypeOnboarding, user, nil, nil, nil, nil, nil)
	movementPrompt := BuildSystemInstruction(api.SessionTypeSessionMovement, user, nil, nil, nil, nil, nil)

	// Both currently share the baseline persona, but they are sourced independently
	// (onboarding from its own package) so they can diverge. Sanity-check both build a
	// persona block.
	if !strings.Contains(onboardingPrompt, "### 3. IDENTITY & PERSONA") {
		t.Error("onboarding prompt missing persona")
	}
	if !strings.Contains(movementPrompt, "### 3. IDENTITY & PERSONA") {
		t.Error("movement prompt missing persona")
	}
}
