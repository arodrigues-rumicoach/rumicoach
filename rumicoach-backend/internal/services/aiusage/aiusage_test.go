package aiusage

import (
	"context"
	"testing"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUsageDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-written DDL for the same reason as the balance package's tests: the model's
	// Postgres column types get TEXT affinity under SQLite AutoMigrate and break
	// time.Time scans.
	if err := db.Exec(`CREATE TABLE ai_usage_records (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, kind TEXT NOT NULL, model TEXT NOT NULL,
		ref_type TEXT, ref_id TEXT,
		input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0,
		input_text_tokens INTEGER DEFAULT 0, output_text_tokens INTEGER DEFAULT 0,
		input_audio_tokens INTEGER DEFAULT 0, output_audio_tokens INTEGER DEFAULT 0,
			input_video_tokens INTEGER DEFAULT 0, output_video_tokens INTEGER DEFAULT 0,
		stt_model TEXT, stt_input_tokens INTEGER DEFAULT 0, stt_output_tokens INTEGER DEFAULT 0, stt_total_tokens INTEGER DEFAULT 0,
			tts_model TEXT, tts_input_tokens INTEGER DEFAULT 0, tts_output_tokens INTEGER DEFAULT 0, tts_total_tokens INTEGER DEFAULT 0,
			cost_micros BIGINT, price_version TEXT,
		created_at DATETIME)`).Error; err != nil {
		t.Fatalf("ddl failed: %v", err)
	}
	database.DB = db
}

func rows(t *testing.T) []models.AIUsageRecord {
	t.Helper()
	var out []models.AIUsageRecord
	if err := database.DB.Find(&out).Error; err != nil {
		t.Fatalf("failed to read records: %v", err)
	}
	return out
}

func TestWritePricesAConfiguredModel(t *testing.T) {
	setupUsageDB(t)

	Write(context.Background(), Record{
		UserID: "u1", Kind: models.AIUsageRecap, Model: "gemini-3.5-flash",
		RefType: models.AIUsageRefSession, RefID: "s1",
		InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_100_000,
	})

	got := rows(t)
	if len(got) != 1 {
		t.Fatalf("rows written: got %d, want 1", len(got))
	}
	r := got[0]
	// gemini-3.5-flash: $1.50 in + $9.00 out on a million of each = $10.50.
	if r.CostMicros == nil || *r.CostMicros != 10_500_000 {
		t.Errorf("cost: got %v, want 10500000", r.CostMicros)
	}
	// The price-table revision is stored so the row stays explainable after the
	// numbers in prices.go change.
	if r.PriceVersion == nil || *r.PriceVersion != PriceTableVersion {
		t.Errorf("price version: got %v, want %q", r.PriceVersion, PriceTableVersion)
	}
	// TotalTokens is the provider's own total (thinking tokens included), not the sum.
	if r.TotalTokens != 2_100_000 {
		t.Errorf("total tokens: got %d, want 2100000", r.TotalTokens)
	}
	if r.RefType == nil || *r.RefType != models.AIUsageRefSession || r.RefID == nil || *r.RefID != "s1" {
		t.Errorf("reference not stored: type=%v id=%v", r.RefType, r.RefID)
	}
}

// An unpriced model still has to be recorded. The tokens and the model id are what
// make the row repriceable later; dropping it would lose the spend permanently.
func TestWriteRecordsUnpricedModelWithNullCost(t *testing.T) {
	setupUsageDB(t)

	Write(context.Background(), Record{
		UserID: "u1", Kind: models.AIUsageMessage, Model: "unpriced-model",
		InputTokens: 500, OutputTokens: 100,
	})

	got := rows(t)
	if len(got) != 1 {
		t.Fatalf("rows written: got %d, want 1", len(got))
	}
	if got[0].CostMicros != nil {
		t.Errorf("cost: got %v, want NULL for an unpriced model", *got[0].CostMicros)
	}
	if got[0].InputTokens != 500 || got[0].OutputTokens != 100 {
		t.Errorf("tokens not recorded: %+v", got[0])
	}
}

// TotalTokens is allowed to arrive zero — some providers omit it — and must not then
// read zero while the other two columns are populated.
func TestWriteFallsBackToTheTokenSum(t *testing.T) {
	setupUsageDB(t)

	Write(context.Background(), Record{
		UserID: "u1", Kind: models.AIUsageReview, Model: "m",
		InputTokens: 700, OutputTokens: 300,
	})

	if got := rows(t)[0].TotalTokens; got != 1000 {
		t.Errorf("total tokens: got %d, want 1000 (input+output fallback)", got)
	}
}

func TestWriteDropsEmptyRecords(t *testing.T) {
	setupUsageDB(t)

	// No tokens at all: a call that spent nothing, or failed before reaching the
	// provider. Nothing to account for.
	Write(context.Background(), Record{UserID: "u1", Kind: models.AIUsageRecap, Model: "m"})
	// No user: nothing to attribute it to.
	Write(context.Background(), Record{Kind: models.AIUsageRecap, Model: "m", InputTokens: 10})

	if got := rows(t); len(got) != 0 {
		t.Errorf("rows written: got %d, want 0 — %+v", len(got), got)
	}
}

// Metering must never be the reason a session close or a message reply fails, so a
// broken table is swallowed rather than propagated.
func TestWriteSurvivesABrokenTable(t *testing.T) {
	setupUsageDB(t)
	database.DB.Exec(`DROP TABLE ai_usage_records`)

	Write(context.Background(), Record{
		UserID: "u1", Kind: models.AIUsageSession, Model: "m", InputTokens: 10,
	})
	// Reaching here without a panic is the assertion.
}
