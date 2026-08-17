package chat

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The recap is written in the user's language, so it is routinely non-ASCII. Truncating
// by bytes would split a multi-byte rune and store invalid UTF-8 that renders as a
// replacement character in the sessions list.
func TestTruncateRecap(t *testing.T) {
	short := "Exploraste o que te trava na carreira e percebeste que estavas à espera de permissão."
	if got := truncateRecap(short); got != short {
		t.Errorf("a short recap must pass through unchanged, got %q", got)
	}

	if got := truncateRecap("  spaced out  "); got != "spaced out" {
		t.Errorf("recap should be trimmed, got %q", got)
	}

	// Accented, sentence-heavy text well over the cap.
	long := strings.Repeat("Percebeste que a tua energia sobe quando crias oportunidades. ", 20)
	got := truncateRecap(long)
	if utf8.RuneCountInString(got) > recapMaxChars {
		t.Errorf("truncated recap is %d runes, want <= %d", utf8.RuneCountInString(got), recapMaxChars)
	}
	if !utf8.ValidString(got) {
		t.Error("truncated recap is not valid UTF-8")
	}
	if !strings.HasSuffix(got, ".") {
		t.Errorf("should have cut on a sentence boundary, got tail %q", got[max(0, len(got)-40):])
	}

	// No sentence boundary in range: cut on a word break and mark the elision.
	noStops := strings.Repeat("palavra ", 200)
	got = truncateRecap(noStops)
	if utf8.RuneCountInString(got) > recapMaxChars {
		t.Errorf("truncated recap is %d runes, want <= %d", utf8.RuneCountInString(got), recapMaxChars)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a mid-sentence cut should be marked with an ellipsis, got tail %q", got[max(0, len(got)-20):])
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Error("ellipsis should follow the word, not a trailing space")
	}

	// A single unbroken run of CJK (no spaces at all) must still be cut safely.
	cjk := strings.Repeat("你", 600)
	got = truncateRecap(cjk)
	if utf8.RuneCountInString(got) > recapMaxChars+1 { // +1 for the ellipsis
		t.Errorf("CJK recap is %d runes, want <= %d", utf8.RuneCountInString(got), recapMaxChars+1)
	}
	if !utf8.ValidString(got) {
		t.Error("CJK truncation produced invalid UTF-8")
	}
}
