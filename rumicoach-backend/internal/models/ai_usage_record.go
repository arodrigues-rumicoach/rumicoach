package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AIUsageKind identifies which piece of the product spent the tokens.
const (
	// AIUsageSession is a live voice session (Gemini Live, whole connection).
	AIUsageSession = "session"
	// AIUsageMessage is one companion turn: the chat tool loop plus any
	// transcription and speech synthesis it needed.
	AIUsageMessage = "message"
	// AIUsageRecap is the short user-facing session summary.
	AIUsageRecap = "recap"
	// AIUsageReview is the internal QA review that grades the coach.
	AIUsageReview = "review"
	// AIUsageRecommendation is the request_recommendations agent.
	AIUsageRecommendation = "recommendation"
)

// AIUsageRecord reference types. There is deliberately no foreign key: both
// referenced tables outlive their content differently (a session row is redacted,
// a channel message is deleted outright), so a join must tolerate the target
// being gone or emptied.
const (
	AIUsageRefSession = "communication_session"
	AIUsageRefMessage = "channel_message"
)

// AIUsageRecord is the cost ledger: one row per call to the AI provider, whatever
// part of the product made it, carrying the token spend and what it cost us. No
// content, ever.
//
// It answers a different question from the ledger it sits beside, and the two should
// not be confused:
//
//   - balance_transactions — what the USER pays and holds (signed seconds, must
//     satisfy balance_seconds = SUM(amount_seconds)). Currency.
//   - this table — what serving it COST US. Measurement.
//
// A charged operation therefore writes two rows: one here (cost) and one in
// balance_transactions (price). A failed call writes only here: it burned tokens and
// produced nothing to charge for.
//
// (A third ledger, channel_usage_records, used to sit between them recording message
// EVENTS; it lost its last reader and is legacy-orphaned now, purged like goals.)
//
// Rows are NEVER deleted by retention sweeps or content erasure — same rule as
// balance_transactions. The QA account reset removes them with everything else.
type AIUsageRecord struct {
	ID     string `gorm:"primaryKey;type:text"`
	UserID string `gorm:"not null;type:text;index:idx_ai_usage_user_created,priority:1"`
	// Kind is one of the AIUsage* constants above — which part of the product spent this.
	Kind string `gorm:"not null;type:text;index"`
	// Model is the provider model id as called. Stored per row because it changes
	// underneath us and the price depends on it.
	Model string `gorm:"not null;type:text"`
	// RefType/RefID point at what the call was for (a session, a message). Nullable:
	// some calls belong to no durable row.
	RefType *string `gorm:"type:text"`
	RefID   *string `gorm:"type:text;index"`

	InputTokens  int `gorm:"default:0"`
	OutputTokens int `gorm:"default:0"`
	// TotalTokens is the provider's totalTokenCount, which also covers thinking
	// tokens that the output count excludes on Gemini 3. Cost is computed from the
	// input/output split; this is the honest total for reporting.
	TotalTokens int `gorm:"default:0"`

	// One billable event can span more than one model: answering a voice note costs
	// transcription, then the chat turn, then speech synthesis, on three models at
	// three different prices. Model/InputTokens/OutputTokens/TotalTokens above are the
	// PRIMARY model's spend; these two groups carry the auxiliary ones, each with the
	// model that earned it so it can be priced on its own terms.
	//
	// Empty model means the group is unused, which is most rows. The row's full spend
	// is the three groups added together — TotalTokens alone is not the row's total.
	// CostMicros already accounts for all three.
	STTModel        string `gorm:"type:text"`
	STTInputTokens  int    `gorm:"default:0"`
	STTOutputTokens int    `gorm:"default:0"`
	STTTotalTokens  int    `gorm:"default:0"`

	TTSModel        string `gorm:"type:text"`
	TTSInputTokens  int    `gorm:"default:0"`
	TTSOutputTokens int    `gorm:"default:0"`
	TTSTotalTokens  int    `gorm:"default:0"`

	// Per-modality counts, populated only by the live voice sessions, which are the
	// only caller that gets them. Audio tokens are priced differently from text, so
	// a single input/output pair cannot express a voice session's cost.
	InputTextTokens   int `gorm:"default:0"`
	OutputTextTokens  int `gorm:"default:0"`
	InputAudioTokens  int `gorm:"default:0"`
	OutputAudioTokens int `gorm:"default:0"`
	InputVideoTokens  int `gorm:"default:0"`
	OutputVideoTokens int `gorm:"default:0"`

	// CostMicros is the computed cost in millionths of a US dollar. NULL means the
	// price table does not know this model — the tokens are still recorded, so the row
	// can be priced retroactively from the model id and the counts beside it. Never a
	// float: money in binary floating point does not add up.
	CostMicros *int64 `gorm:"type:bigint"`
	// PriceVersion is the revision of the price table that produced CostMicros
	// (aiusage.PriceTableVersion). Stored rather than the individual rates because it
	// identifies all of them at once, including the per-modality split — which is what
	// makes a cost recorded today still explainable after the prices change. NULL
	// exactly when CostMicros is.
	PriceVersion *string `gorm:"type:text"`

	CreatedAt time.Time `gorm:"autoCreateTime;type:timestamp with time zone;index:idx_ai_usage_user_created,priority:2,sort:desc"`
}

func (r *AIUsageRecord) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return
}
