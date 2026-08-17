package aiusage

// Model prices, in micro-dollars (millionths of a USD) per million tokens.
//
// This file is the single source of truth for what the AI costs us. It is checked in
// rather than configured because a price is a fact about a model, it changes rarely,
// and it should move through review like any other fact the billing depends on.
//
// ─── Where these came from ────────────────────────────────────────────────────
//
// Read from the official Gemini API pricing page on 2026-08-17, paid tier:
//   https://ai.google.dev/gemini-api/docs/pricing
//
// PriceTableVersion below stamps every row that was priced with this table, so a cost
// recorded today stays explainable after these numbers change. Bump it whenever a
// price here changes — that is what makes an old row auditable rather than merely old.
//
// ─── What is NOT modelled ─────────────────────────────────────────────────────
//
//   - Context-length tiers. gemini-3.1-pro-preview doubles its input rate above 200k
//     tokens ($2.00 → $4.00) and raises output ($12.00 → $18.00). Only the
//     recommendation agent uses that model and its prompts are a topic, a query and a
//     short context — nowhere near the threshold. A prompt that did cross it would be
//     under-priced here.
//   - The per-minute alternative rates the Live API also publishes. We count tokens,
//     so we price tokens.
//   - Introductory rates that expire on a date. gemini-3.7-flash is launch-priced at
//     half rate until 2026-12-31 and doubles on 2027-01-01. This table has no notion
//     of an effective date, so that is a diary entry, not something the code will
//     notice: on 2027-01-01 the workhorse model silently starts costing twice what
//     every row here says. See the entry below.
//   - Vertex vs AI Studio differences, and any negotiated rate. If those diverge this
//     table is what has to learn about it.
//
// Preview models (the -preview suffix) are exactly the ones whose prices move, and
// three of the five below are previews. Re-check this page when a bill surprises you.

// PriceTableVersion identifies this revision of the table. Stored on every priced row.
const PriceTableVersion = "2026-08-17"

// Price is what one model costs per million tokens, split by modality because the
// models we use charge audio and text very differently — the Live model's audio output
// is $12.00 against $4.50 for text, and a voice session is almost entirely audio, so
// pricing it with a single output rate would understate it nearly threefold.
//
// For text-only models the audio rates are set equal to the text rates rather than
// left zero: an unexpected audio token should be charged something plausible, not
// silently priced at nothing.
type Price struct {
	InputTextMicrosPerMTok   int64
	InputAudioMicrosPerMTok  int64
	InputVideoMicrosPerMTok  int64
	OutputTextMicrosPerMTok  int64
	OutputAudioMicrosPerMTok int64
}

// text builds a Price for a model with no separate audio rate.
func text(in, out int64) Price {
	return Price{
		InputTextMicrosPerMTok:   in,
		InputAudioMicrosPerMTok:  in,
		InputVideoMicrosPerMTok:  in,
		OutputTextMicrosPerMTok:  out,
		OutputAudioMicrosPerMTok: out,
	}
}

// usd converts a published dollars-per-million-tokens price to micro-dollars, so the
// table below reads as the numbers actually printed on Google's pricing page.
func usd(dollarsPerMTok float64) int64 {
	return int64(dollarsPerMTok * 1_000_000)
}

// prices maps a provider model id to its price. Every model this codebase can call
// should have an entry; anything missing records its tokens with a NULL cost rather
// than a guess, and `TestEveryConfiguredModelIsPriced` is what notices.
var prices = map[string]Price{
	// The workhorse: companion chat, voice-note transcription, session recaps and the
	// QA review all default to it (GEMINI_COMPANION_MODEL, GEMINI_TRANSCRIBE_MODEL,
	// GEMINI_REVIEW_MODEL).
	//
	// LAUNCH PRICING, EXPIRES 2026-12-31. From 2027-01-01 these become $1.50 in /
	// $7.50 out — the same input rate as the 3.5-flash it replaced, and a bit under
	// its output. Nothing here enforces the date: update these two numbers and bump
	// PriceTableVersion on 2027-01-01, or every row recorded after it understates the
	// cost by half.
	"gemini-3.7-flash": text(usd(0.75), usd(3.75)),

	// The previous workhorse, kept priced because GEMINI_COMPANION_MODEL /
	// GEMINI_TRANSCRIBE_MODEL / GEMINI_REVIEW_MODEL can still be pointed back at it.
	"gemini-3.5-flash": text(usd(1.50), usd(9.00)),

	// The recommendation agent (GEMINI_RECOMMENDATION_MODEL). Rates for prompts
	// <= 200k tokens — see the note above.
	"gemini-3.1-pro-preview": text(usd(2.00), usd(12.00)),

	// The live voice sessions (GEMINI_LIVE_MODEL). The one model where the modality
	// split matters: audio is 4x the text rate on input and ~2.7x on output.
	"gemini-3.1-flash-live-preview": {
		InputTextMicrosPerMTok:   usd(0.75),
		InputAudioMicrosPerMTok:  usd(3.00),
		InputVideoMicrosPerMTok:  usd(1.00),
		OutputTextMicrosPerMTok:  usd(4.50),
		OutputAudioMicrosPerMTok: usd(12.00),
	},

	// Voice-note synthesis (GEMINI_TTS_MODEL): text in, audio out, and the audio is
	// expensive — $20.00 per million output tokens.
	"gemini-3.1-flash-tts-preview": {
		InputTextMicrosPerMTok:   usd(1.00),
		InputAudioMicrosPerMTok:  usd(1.00),
		InputVideoMicrosPerMTok:  usd(1.00),
		OutputTextMicrosPerMTok:  usd(20.00),
		OutputAudioMicrosPerMTok: usd(20.00),
	},

	// The alternative live model named in config.go's comment on GEMINI_LIVE_MODEL.
	// Priced here so switching to it does not silently stop costing anything.
	"gemini-live-2.5-flash-native-audio": {
		InputTextMicrosPerMTok:   usd(0.50),
		InputAudioMicrosPerMTok:  usd(3.00),
		InputVideoMicrosPerMTok:  usd(3.00),
		OutputTextMicrosPerMTok:  usd(2.00),
		OutputAudioMicrosPerMTok: usd(12.00),
	},
	// Same model under the id the pricing page lists it as.
	"gemini-2.5-flash-native-audio-preview-12-2025": {
		InputTextMicrosPerMTok:   usd(0.50),
		InputAudioMicrosPerMTok:  usd(3.00),
		InputVideoMicrosPerMTok:  usd(3.00),
		OutputTextMicrosPerMTok:  usd(2.00),
		OutputAudioMicrosPerMTok: usd(12.00),
	},
}

// PriceFor returns the price for a model, and whether there is one.
//
// A model with no entry is not priced at a default: it records its tokens with a NULL
// cost, which is repriceable later from the model id and counts on the row, whereas a
// guessed rate produces a cost column that looks authoritative and is wrong.
func PriceFor(model string) (Price, bool) {
	p, ok := prices[model]
	return p, ok
}

// CostOf returns what a whole event cost, in micro-dollars, adding up the primary
// model and any auxiliary ones — a voice note is transcribed, answered and spoken on
// three models at three prices, and all three are its cost.
//
// ok is false when ANY model carrying tokens has no price, and then nothing is
// recorded rather than a partial sum. A cost that silently omits one of three models
// is worse than an admitted NULL: the NULL is visibly incomplete and repriceable from
// the counts and model ids on the row, whereas an understatement reads as fact.
func CostOf(rec Record) (int64, bool) {
	var total int64

	if rec.InputTokens != 0 || rec.OutputTokens != 0 {
		price, ok := PriceFor(rec.Model)
		if !ok {
			return 0, false
		}
		total += price.cost(rec)
	}

	for _, aux := range []AuxSpend{rec.STT, rec.TTS} {
		if !aux.used() {
			continue
		}
		price, ok := PriceFor(aux.Model)
		if !ok {
			return 0, false
		}
		// Priced through the same path as anything else, as a plain input/output
		// call: neither transcription nor synthesis reports a modality split, and
		// for the models that do them the audio and text rates are equal anyway.
		total += price.cost(Record{InputTokens: aux.InputTokens, OutputTokens: aux.OutputTokens})
	}

	return total, true
}

// cost returns what one model's share of a call cost, in micro-dollars. Callers
// pricing a whole event want CostOf, which also covers the auxiliary models.
//
// Tokens are priced by modality where the caller knows the split (the live voice
// sessions report it; nothing else does). Anything the split does not account for —
// all of it, for a caller that reports only totals — is charged at the text rate,
// which is the conservative reading for a text-mostly call and the only sensible
// default when the modality is genuinely unknown.
//
// Rounded down by the integer division, losing at most a millionth of a dollar per
// call. Money is integer micro-dollars throughout: a ledger in binary floating point
// stops adding up exactly at the scale a ledger exists for.
func (p Price) cost(r Record) int64 {
	inText, inAudio, inVideo := int64(r.InputTextTokens), int64(r.InputAudioTokens), int64(r.InputVideoTokens)
	outText, outAudio := int64(r.OutputTextTokens), int64(r.OutputAudioTokens)
	// No model we call emits video and none publishes an output-video rate, so those
	// tokens ride the output text rate. They should always be zero.
	outText += int64(r.OutputVideoTokens)

	// Whatever the modality columns do not account for falls to the text rate. When
	// they are all zero — every caller but the live sessions — this puts the totals
	// there, which is the same thing as having no split at all.
	if rem := int64(r.InputTokens) - (inText + inAudio + inVideo); rem > 0 {
		inText += rem
	}
	if rem := int64(r.OutputTokens) - (outText + outAudio); rem > 0 {
		outText += rem
	}

	total := inText*p.InputTextMicrosPerMTok +
		inAudio*p.InputAudioMicrosPerMTok +
		inVideo*p.InputVideoMicrosPerMTok +
		outText*p.OutputTextMicrosPerMTok +
		outAudio*p.OutputAudioMicrosPerMTok
	return total / 1_000_000
}
