// Package aiusage is the cost ledger for everything the product spends on the AI
// provider: live voice sessions, companion messages, and the background calls that
// used to be invisible (session recaps, QA reviews, the recommendation agent).
//
// One row per provider call, tokens plus what they cost, no content. It measures OUR
// cost; what the user is charged is a separate decision recorded in
// balance_transactions. A call that failed still belongs here — it burned tokens and
// produced nothing to bill for.
//
// Every write is log-and-continue, like the session debit: metering must never be the
// reason a session close or a message reply fails.
package aiusage

import (
	"context"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// Record is one call's spend, as the caller knows it. Cost is not passed in — it is
// computed here from Model and the configured prices, so no call site can invent one.
type Record struct {
	UserID string
	Kind   string
	Model  string
	// RefType/RefID link the call to what it was for; both empty when it belongs to
	// no durable row.
	RefType string
	RefID   string

	InputTokens  int
	OutputTokens int
	// TotalTokens may be left zero: Write falls back to input+output, so the column
	// never reads zero while the other two are populated.
	TotalTokens int

	// Per-modality counts, live voice sessions only. Leave zero elsewhere.
	InputTextTokens   int
	OutputTextTokens  int
	InputAudioTokens  int
	OutputAudioTokens int
	InputVideoTokens  int
	OutputVideoTokens int

	// Auxiliary models this same event also spent on. Answering a voice note runs
	// transcription, the chat turn and speech synthesis on three different models at
	// three different prices; Model above is the primary one and these carry the rest,
	// each priced on its own terms. Leave the model empty when unused.
	STT AuxSpend
	TTS AuxSpend
}

// AuxSpend is one auxiliary model's share of an event.
type AuxSpend struct {
	Model        string
	InputTokens  int
	OutputTokens int
	// TotalTokens may be left zero; it falls back to input+output like the main trio.
	TotalTokens int
}

// used reports whether this group carries anything worth recording or pricing.
func (a AuxSpend) used() bool {
	return a.Model != "" && (a.InputTokens != 0 || a.OutputTokens != 0 || a.TotalTokens != 0)
}

func (a AuxSpend) total() int {
	if a.TotalTokens != 0 {
		return a.TotalTokens
	}
	return a.InputTokens + a.OutputTokens
}

// Write appends one row to the cost ledger.
//
// Never returns an error: there is no caller that could do anything useful with one,
// and every one of them is on a path (session teardown, message reply) where failing
// the operation to record its cost would be the wrong trade. Failures go to the global
// logger, which main.go installs — the AI-calling helpers are free functions with no
// logger of their own, and threading one through them purely for metering would be a
// worse trade than the rare silent line here.
//
// A record with no tokens at all is dropped rather than stored: it carries no
// information and the callers include paths that legitimately spend nothing (a cached
// reply, a call that failed before the provider was reached).
func Write(ctx context.Context, rec Record) {
	if rec.UserID == "" {
		return
	}
	if rec.InputTokens == 0 && rec.OutputTokens == 0 && rec.TotalTokens == 0 &&
		!rec.STT.used() && !rec.TTS.used() {
		return
	}

	total := rec.TotalTokens
	if total == 0 {
		total = rec.InputTokens + rec.OutputTokens
	}

	row := models.AIUsageRecord{
		UserID:            rec.UserID,
		Kind:              rec.Kind,
		Model:             rec.Model,
		InputTokens:       rec.InputTokens,
		OutputTokens:      rec.OutputTokens,
		TotalTokens:       total,
		InputTextTokens:   rec.InputTextTokens,
		OutputTextTokens:  rec.OutputTextTokens,
		InputAudioTokens:  rec.InputAudioTokens,
		OutputAudioTokens: rec.OutputAudioTokens,
		InputVideoTokens:  rec.InputVideoTokens,
		OutputVideoTokens: rec.OutputVideoTokens,

		STTModel:        rec.STT.Model,
		STTInputTokens:  rec.STT.InputTokens,
		STTOutputTokens: rec.STT.OutputTokens,
		STTTotalTokens:  rec.STT.total(),

		TTSModel:        rec.TTS.Model,
		TTSInputTokens:  rec.TTS.InputTokens,
		TTSOutputTokens: rec.TTS.OutputTokens,
		TTSTotalTokens:  rec.TTS.total(),
	}
	if rec.RefType != "" {
		row.RefType = &rec.RefType
	}
	if rec.RefID != "" {
		row.RefID = &rec.RefID
	}

	// An unpriced model still gets its tokens recorded — the row can be priced later,
	// which is only possible because the token counts and the model ids are on it.
	if cost, ok := CostOf(rec); ok {
		version := PriceTableVersion
		row.CostMicros = &cost
		row.PriceVersion = &version
	}

	if err := database.DB.WithContext(ctx).Create(&row).Error; err != nil {
		zap.L().Warn("aiusage: failed to record AI spend", zap.Error(err),
			zap.String("kind", rec.Kind), zap.String("model", rec.Model),
			zap.String("userID", rec.UserID))
	}
}
