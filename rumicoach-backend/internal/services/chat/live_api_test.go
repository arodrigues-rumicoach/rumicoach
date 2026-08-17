package chat

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

func TestChatSession_LogHistoryAndFlushUserTranscript(t *testing.T) {
	session := &ChatSession{}

	// Test logHistory with "User" mapping to "USER"
	session.logHistory("User", "Hello there!")

	// Test logHistory with "AI"
	session.logHistory("AI", "Hi, how can I help you?")

	// Test logHistory with "Server to Client"
	session.logHistory("Server to Client", `{"some":"payload"}`)

	// Test flushUserTranscript with partial and accumulated transcript
	session.accumulatedUserTranscript = "How are"
	session.currentPartialTranscript = "you today?"
	session.flushUserTranscript()

	history := session.getHistoryLog()
	lines := strings.Split(history, "\n")

	if len(lines) != 4 {
		t.Fatalf("Expected 4 lines of history, got %d. History: \n%s", len(lines), history)
	}

	// Helper to check line components: e.g., "[2026-06-07 21:22:14] [USER] Hello there!"
	checkLine := func(line string, expectedPrefix, expectedContent string) {
		// Line format: [YYYY-MM-DD HH:MM:SS] [Prefix] Content
		parts := strings.SplitN(line, " ", 3) // split after time: [YYYY-MM-DD, HH:MM:SS, remainder]
		if len(parts) < 3 {
			t.Errorf("Line %q is not in expected timestamped format", line)
			return
		}
		remainder := parts[2] // remainder: [Prefix] Content
		expectedStart := "[" + expectedPrefix + "] "
		if !strings.HasPrefix(remainder, expectedStart) {
			t.Errorf("Expected line to start with %q, got %q in line %q", expectedStart, remainder, line)
		}
		actualContent := strings.TrimPrefix(remainder, expectedStart)
		if actualContent != expectedContent {
			t.Errorf("Expected content to be %q, got %q in line %q", expectedContent, actualContent, line)
		}
	}

	checkLine(lines[0], "USER", "Hello there!")
	checkLine(lines[1], "AI", "Hi, how can I help you?")
	checkLine(lines[2], "SERVER TO CLIENT", `{"some":"payload"}`)
	checkLine(lines[3], "USER", "How are you today?")
}

func TestCleanLeakedToolCalls(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		raw      string
	}{
		{
			input:    "I saved it. save_memory{category: \"insight\", content: \"hello\"} Good job.",
			expected: "I saved it. Good job.",
			raw:      "I saved it.  Good job.",
		},
		{
			input:    "call:save_memory(category=\"obstacles\", content=\"something\")",
			expected: "",
			raw:      "",
		},
		{
			input:    "Do you think update_wheel_of_life{health: 8} makes sense?",
			expected: "Do you think makes sense?",
			raw:      "Do you think  makes sense?",
		},
		{
			input:    "No, it is health (which is 6).",
			expected: "No, it is health (which is 6).",
			raw:      "No, it is health (which is 6).",
		},
		{
			input:    "Hello. [P=2] Are you ready?",
			expected: "Hello. [P=2] Are you ready?",
			raw:      "Hello. [P=2] Are you ready?",
		},
	}

	for _, tt := range tests {
		got := CleanLeakedToolCalls(tt.input)
		if got != tt.expected {
			t.Errorf("CleanLeakedToolCalls(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
		gotRaw := CleanLeakedToolCallsRaw(tt.input)
		if gotRaw != tt.raw {
			t.Errorf("CleanLeakedToolCallsRaw(%q) = %q, expected %q", tt.input, gotRaw, tt.raw)
		}
	}
}

func TestOpenToolRegexPatterns(t *testing.T) {
	openBraceTests := []struct {
		input  string
		expect bool
	}{
		{"Hello. save_memory{category: \"insight\"", true},
		{"Hello. save_memory{category: \"insight\"}", false},
		{"some_identifier{", true},
		{"some_identifier{}", false},
	}

	for _, tt := range openBraceTests {
		got := openBraceRegex.MatchString(tt.input)
		if got != tt.expect {
			t.Errorf("openBraceRegex.MatchString(%q) = %v, expected %v", tt.input, got, tt.expect)
		}
	}

	openToolTests := []struct {
		input  string
		expect bool
	}{
		{"Hello. save_memory(category: \"insight\"", true},
		{"Hello. save_memory(category: \"insight\")", false},
		{"Hello. save_memory[category: \"insight\"", true},
		{"Hello. save_memory[category: \"insight\"]", false},
		{"Hello. update_wheel_of_life(score: 8", true},
		{"Hello. update_wheel_of_life(score: 8)", false},
	}

	for _, tt := range openToolTests {
		got := openToolRegex.MatchString(tt.input)
		if got != tt.expect {
			t.Errorf("openToolRegex.MatchString(%q) = %v, expected %v", tt.input, got, tt.expect)
		}
	}
}

func TestControlMarkerParsing(t *testing.T) {
	// Screen markers decode to the right screen name.
	screenCases := map[string]string{"◆▣": "memories", "◆▤": "session"}
	for marker, want := range screenCases {
		text := "Here you go. " + marker + " All set."
		m := screenSymbolRegex.FindStringSubmatch(text)
		if len(m) < 2 {
			t.Fatalf("screen marker %q not matched in %q", marker, text)
		}
		if got := screenSymbolToName[m[1]]; got != want {
			t.Errorf("screen marker %q decoded to %q, want %q", marker, got, want)
		}
	}

	// Pause markers decode to the right number of seconds (block height == seconds).
	pauseCases := map[string]int{"●▁": 1, "●▂": 2, "●▄": 4, "●█": 8}
	for marker, want := range pauseCases {
		text := "Take a breath. " + marker + " Now continue."
		m := pauseSymbolRegex.FindStringSubmatch(text)
		if len(m) < 2 {
			t.Fatalf("pause marker %q not matched in %q", marker, text)
		}
		if got := pauseSymbolToSeconds[m[1]]; got != want {
			t.Errorf("pause marker %q decoded to %d, want %d", marker, got, want)
		}
	}

	// Plain coaching text must NOT trip the marker regexes (no false positives).
	for _, clean := range []string{"Let's focus on what matters.", "The diamond ◆ alone is fine.", "A bullet ● by itself too."} {
		if screenSymbolRegex.MatchString(clean) || pauseSymbolRegex.MatchString(clean) {
			t.Errorf("clean text falsely matched a marker: %q", clean)
		}
	}
}

// The model occasionally writes a state-advancing tool call as plain TEXT instead of
// invoking it natively (QA 2026-07-16: five leaked "call:save_ideal_life_vision{vision:..."
// attempts left a session looping in the vision phase). These cases are lifted verbatim
// from that session's transcript.
func TestLeakedToolCallDetectionAndVisionExtraction(t *testing.T) {
	// Leak with NO closing brace, gluing straight into the next sentence ("voluntariadoFico").
	leak1 := `Gratidão por partilhares tudo isso — que visão bonita. call:save_ideal_life_vision{vision:A Filipa visualiza uma vida familiar rica, onde é mãe a tempo inteiro de dois filhos, casada com o André, vivendo numa casa com quintal (flores e horta). Dá grande importância à presença, a estar com amigos e família, e dedicar-se a hobbies e voluntariadoFico feliz por teres uma visão tão clara, Filipa!`
	// Leak gluing at a sentence terminator (".Pode").
	leak2 := `Grato por partilhares. call:save_ideal_life_vision{vision:A Filipa deseja uma vida plena e presente, casada com o André, com dois filhos e uma casa com quintal. Em suma, quer deixar de correr e viver alinhada com o coração.Pode dizer-se que esta visão reflete o que mais te importa?`

	for i, leak := range []string{leak1, leak2} {
		if got := leakedTransitionToolName(leak); got != "save_ideal_life_vision" {
			t.Fatalf("case %d: leaked tool not detected, got %q", i+1, got)
		}
		vision := extractLeakedVision(leak)
		if vision == "" {
			t.Fatalf("case %d: no vision extracted", i+1)
		}
		if !strings.HasPrefix(vision, "A Filipa") {
			t.Errorf("case %d: vision should start with the argument text, got %q", i+1, vision)
		}
		if strings.Contains(vision, "Fico feliz") || strings.Contains(vision, "Pode dizer-se") {
			t.Errorf("case %d: vision swallowed the model's next sentence: %q", i+1, vision)
		}
	}
	if strings.Contains(extractLeakedVision(leak1), "voluntariadoFico") {
		t.Error("glue point not cut: vision still contains 'voluntariadoFico'")
	}
	if !strings.Contains(extractLeakedVision(leak1), "(flores e horta)") {
		t.Error("parenthesised content inside the vision must survive extraction")
	}

	// The transcript cleaner must strip the unterminated leak so raw code never reaches
	// the client transcript.
	cleaned := CleanLeakedToolCalls(leak1)
	if strings.Contains(cleaned, "call:") || strings.Contains(cleaned, "{") {
		t.Errorf("cleaner left tool syntax in transcript: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Gratidão por partilhares") {
		t.Errorf("cleaner dropped the genuine speech before the leak: %q", cleaned)
	}

	// Native-call turns have no tool syntax in text — no false positives.
	if got := leakedTransitionToolName("Obrigado por partilhares essa visão tão bonita."); got != "" {
		t.Errorf("clean text falsely detected as leak: %q", got)
	}
	// A terminated call followed by real speech keeps that speech after cleaning.
	terminated := `Que visão bonita. save_ideal_life_vision{vision:Uma vida calma.} Vamos continuar.`
	if cleaned := CleanLeakedToolCalls(terminated); !strings.Contains(cleaned, "Vamos continuar.") {
		t.Errorf("speech after a terminated call must survive: %q", cleaned)
	}
}

// The Vision session's emotional closing must not be completed right after the insight is saved:
// the permission question, personalized synthesis, and clarity check (≥2 spoken turns)
// still have to happen. An early complete_current_task is rejected exactly once.
func TestEmotionalClosingEarlyCompleteGuard(t *testing.T) {
	state := string(models.StateVisionEmotionalClosing)
	session := &ChatSession{
		logger: zap.NewNop(),
		User:   &models.User{State: &state},
	}
	session.closingInsightSaved = true
	session.closingTurnsAfterInsight = 0

	out, err := session.handleCompleteCurrentTask(map[string]interface{}{"current_state": state})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "REJECTED") {
		t.Fatalf("early complete_current_task should be rejected, got %q", out)
	}
	if !session.closingEarlyCompleteRejected {
		t.Error("rejection must be recorded so the retry always passes")
	}

	// With the synthesis dialogue delivered (2 spoken turns), the guard lets the call
	// through to the transition logic (which then fails on the missing DB — that failure
	// mode proves the guard itself no longer blocks).
	session2 := &ChatSession{
		logger: zap.NewNop(),
		User:   &models.User{State: &state},
	}
	session2.closingInsightSaved = true
	session2.closingTurnsAfterInsight = 2
	out2, _ := session2.handleCompleteCurrentTask(map[string]interface{}{"current_state": state})
	if strings.Contains(out2, "REJECTED") {
		t.Fatalf("complete_current_task after the synthesis dialogue must not be rejected, got %q", out2)
	}
}
