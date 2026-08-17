package aiusage

import "testing"

// The models this table must cover, and what each is for. A literal list rather than
// a lookup: the guards that catch a CHANGED default live in the packages that own
// those defaults (companion and chat both have one), because only they can see the
// new value. This one documents what the table is meant to contain.
func TestEveryModelWeCallIsPriced(t *testing.T) {
	for _, c := range []struct{ what, model string }{
		{"companion chat, transcription, session recap, QA review", "gemini-3.5-flash"},
		{"recommendation agent", "gemini-3.1-pro-preview"},
		{"live voice session", "gemini-3.1-flash-live-preview"},
		{"voice-note synthesis", "gemini-3.1-flash-tts-preview"},
		{"alternative live model named in config.go", "gemini-live-2.5-flash-native-audio"},
	} {
		if _, ok := PriceFor(c.model); !ok {
			t.Errorf("%s uses %q, which has no price in prices.go", c.what, c.model)
		}
	}
}

func TestPriceForUnknownModel(t *testing.T) {
	if _, ok := PriceFor("some-model-nobody-priced"); ok {
		t.Error("unknown model reported a price")
	}
	if _, ok := PriceFor(""); ok {
		t.Error("empty model reported a price")
	}
}

// A caller that reports only totals — everything but the live sessions — is priced at
// the text rate on both sides.
func TestCostWithoutAModalitySplit(t *testing.T) {
	p, ok := PriceFor("gemini-3.5-flash") // $1.50 in, $9.00 out
	if !ok {
		t.Fatal("gemini-3.5-flash is not priced")
	}

	// 1M in + 1M out = $1.50 + $9.00 = $10.50 = 10_500_000 micro-dollars.
	got := p.cost(Record{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	if got != 10_500_000 {
		t.Errorf("a million of each: got %d, want 10500000", got)
	}
	if got := p.cost(Record{}); got != 0 {
		t.Errorf("no tokens: got %d, want 0", got)
	}
	// $1.50/Mtok is 1.5 micro-dollars per token, and the integer division floors it.
	if got := p.cost(Record{InputTokens: 1}); got != 1 {
		t.Errorf("a single token at $1.50/Mtok: got %d, want 1", got)
	}
	// Below a micro-dollar the floor reaches zero — the live model's text input is
	// $0.75/Mtok, so one token is 0.75 micro-dollars and rounds away. The loss is
	// bounded by one millionth of a dollar per call.
	live, _ := PriceFor("gemini-3.1-flash-live-preview")
	if got := live.cost(Record{InputTokens: 1, InputTextTokens: 1}); got != 0 {
		t.Errorf("a single token at $0.75/Mtok: got %d, want 0", got)
	}
}

// The reason the split exists: the Live model charges audio output $12.00 against
// $4.50 for text. Pricing a voice session — which is almost entirely audio — at the
// text rate would understate it nearly threefold.
func TestCostPricesAudioApartFromText(t *testing.T) {
	p, ok := PriceFor("gemini-3.1-flash-live-preview")
	if !ok {
		t.Fatal("the live model is not priced")
	}

	audio := p.cost(Record{
		InputTokens: 1_000_000, InputAudioTokens: 1_000_000,
		OutputTokens: 1_000_000, OutputAudioTokens: 1_000_000,
	})
	// $3.00 in + $12.00 out = $15.00.
	if audio != 15_000_000 {
		t.Errorf("all-audio call: got %d, want 15000000", audio)
	}

	text := p.cost(Record{
		InputTokens: 1_000_000, InputTextTokens: 1_000_000,
		OutputTokens: 1_000_000, OutputTextTokens: 1_000_000,
	})
	// $0.75 in + $4.50 out = $5.25.
	if text != 5_250_000 {
		t.Errorf("all-text call: got %d, want 5250000", text)
	}
	if audio <= text {
		t.Errorf("audio (%d) should cost more than text (%d); the split is not being applied", audio, text)
	}
}

// Tokens the modality columns do not account for still have to be charged. Silently
// dropping the remainder would under-report exactly the calls whose reporting is
// incomplete.
func TestCostChargesTheUnclassifiedRemainder(t *testing.T) {
	p, _ := PriceFor("gemini-3.1-flash-live-preview")

	// 1M input of which only 600k is classified as audio: the other 400k is charged
	// at the text rate. 600000*$3.00/M + 400000*$0.75/M = $1.80 + $0.30 = $2.10.
	got := p.cost(Record{InputTokens: 1_000_000, InputAudioTokens: 600_000})
	if got != 2_100_000 {
		t.Errorf("partial split: got %d, want 2100000", got)
	}
}

// A split larger than the reported total (a provider quirk, or a caller filling only
// the modality fields) must still price what it was told about rather than going
// negative on the remainder.
func TestCostToleratesASplitExceedingTheTotal(t *testing.T) {
	p, _ := PriceFor("gemini-3.1-flash-live-preview")

	got := p.cost(Record{InputTokens: 0, InputAudioTokens: 1_000_000})
	if got != 3_000_000 {
		t.Errorf("split with no total: got %d, want 3000000", got)
	}
}

// The published price is dollars per million tokens; the table stores micro-dollars.
// A slip of 1000x here would be invisible in every other test, which all use round
// numbers that stay round.
func TestPublishedRatesConvertExactly(t *testing.T) {
	p, _ := PriceFor("gemini-3.5-flash")
	if p.InputTextMicrosPerMTok != 1_500_000 {
		t.Errorf("$1.50/Mtok stored as %d, want 1500000", p.InputTextMicrosPerMTok)
	}
	if p.OutputTextMicrosPerMTok != 9_000_000 {
		t.Errorf("$9.00/Mtok stored as %d, want 9000000", p.OutputTextMicrosPerMTok)
	}
	// A text-only model must not price audio at zero.
	if p.InputAudioMicrosPerMTok == 0 || p.OutputAudioMicrosPerMTok == 0 {
		t.Error("a text-only model prices audio at zero; unexpected audio would be free")
	}
}
