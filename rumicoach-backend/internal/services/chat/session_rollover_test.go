package chat

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRolloverTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created tables (like balance_test.go): the models' Postgres column types
	// ("timestamp with time zone") get TEXT affinity under SQLite AutoMigrate and
	// break time.Time scans; DATETIME columns scan fine.
	for _, ddl := range []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			state TEXT,
			balance_seconds INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE communication_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			duration INTEGER DEFAULT 0,
			language TEXT,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			input_text_tokens INTEGER DEFAULT 0,
			output_text_tokens INTEGER DEFAULT 0,
			input_audio_tokens INTEGER DEFAULT 0,
			output_audio_tokens INTEGER DEFAULT 0,
			input_video_tokens INTEGER DEFAULT 0,
			output_video_tokens INTEGER DEFAULT 0,
			deepgram_duration REAL DEFAULT 0,
			stt_service TEXT,
			session_type TEXT,
			transcript TEXT,
			ai_notes TEXT,
			ai_evaluation NUMERIC,
			user_evaluation NUMERIC,
			user_feedback TEXT,
			user_session_insight TEXT,
			session_summary TEXT,
			recap TEXT,
			recap_title TEXT
		)`,
		`CREATE TABLE balance_transactions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			amount_seconds INTEGER NOT NULL,
			balance_after INTEGER NOT NULL,
			session_id TEXT UNIQUE,
			session_type TEXT,
			product TEXT,
			reference_id TEXT UNIQUE,
			description TEXT,
			created_at DATETIME
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	database.DB = db
}

func newRolloverTestSession(t *testing.T, state string, sessionType api.SessionType, startedAgo time.Duration) *ChatSession {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO users (id, state, balance_seconds, created_at) VALUES (?, ?, 0, ?)`,
		"user-1", state, time.Now()).Error; err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	lang := "pt-PT"
	s := &ChatSession{
		logger:      zap.NewNop(),
		UserID:      "user-1",
		User:        &models.User{ID: "user-1", State: &state, PreferredLanguage: &lang},
		SessionType: sessionType,
	}
	s.InitDB()
	if s.SessionDB.ID == "" {
		t.Fatal("InitDB did not create a session row")
	}
	// Backdate the row so the ended half is long enough for SessionCountsAsDone.
	start := time.Now().Add(-startedAgo)
	s.SessionDB.StartTime = start
	if err := database.DB.Model(&models.CommunicationSession{}).Where("id = ?", s.SessionDB.ID).
		Update("start_time", start).Error; err != nil {
		t.Fatalf("failed to backdate session: %v", err)
	}
	return s
}

// The onboarding intro hands straight over to Vision on start_planned_session. That
// handover must leave TWO communication_sessions rows — the intro ended, Vision open —
// because the journey gates read typed, ended rows: a single row spanning both halves
// (typed onboarding) left the completed Vision session invisible, so the journey kept
// proposing Vision forever.
func TestIntroHandoverSplitsSessionRows(t *testing.T) {
	setupRolloverTestDB(t)
	s := newRolloverTestSession(t, string(models.StateOnboardingIntro), api.SessionTypeOnboarding, 2*time.Minute)
	// Mirrors the capture at connection start: ONBOARDING_INTRO is first-journey → free.
	s.balanceExempt = true
	introID := s.SessionDB.ID

	if _, err := s.handleStartPlannedSession(nil); err != nil {
		t.Fatalf("handleStartPlannedSession: %v", err)
	}

	var intro models.CommunicationSession
	if err := database.DB.First(&intro, "id = ?", introID).Error; err != nil {
		t.Fatalf("intro row missing: %v", err)
	}
	if intro.SessionType == nil || *intro.SessionType != string(api.SessionTypeOnboarding) {
		t.Errorf("intro row type = %v, want onboarding", intro.SessionType)
	}
	if intro.EndTime == nil {
		t.Error("intro row was not ended at handover")
	}
	if !journey.SessionCountsAsDone(*intro.SessionType, intro.Duration) {
		t.Errorf("ended intro (duration %ds) does not count as done for the journey gates", intro.Duration)
	}

	if s.SessionDB.ID == introID {
		t.Fatal("no new session row was created at handover")
	}
	var vision models.CommunicationSession
	if err := database.DB.First(&vision, "id = ?", s.SessionDB.ID).Error; err != nil {
		t.Fatalf("vision row missing: %v", err)
	}
	if vision.SessionType == nil || *vision.SessionType != string(api.SessionTypeSessionVision) {
		t.Errorf("new row type = %v, want session_vision", vision.SessionType)
	}
	if vision.EndTime != nil {
		t.Error("new row must still be open (Cleanup ends it)")
	}

	// The handover moved the user into Vision's opening state; both halves of the
	// opening pair are free, so the intro row must be marked free rather than debited.
	if s.User.State == nil || *s.User.State != string(models.StateVisionIdealLife) {
		t.Errorf("user state = %v, want VISION_IDEAL_LIFE", s.User.State)
	}
	var txs []models.BalanceTransaction
	database.DB.Find(&txs)
	if len(txs) != 1 {
		t.Fatalf("intro handover produced %d balance transactions, want 1 (the free marker)", len(txs))
	}
	// Zero amount, so the balance is untouched — the row exists only so the usage
	// history can show the session instead of silently omitting it. Recorded rather
	// than skipped because "no row" cannot be told apart from a debit that failed.
	if txs[0].Type != models.BalanceTxSessionFree || txs[0].AmountSeconds != 0 {
		t.Errorf("intro row = %s/%d seconds, want session_free/0", txs[0].Type, txs[0].AmountSeconds)
	}
	if txs[0].SessionID == nil || *txs[0].SessionID != intro.ID {
		t.Errorf("free marker points at %v, want the intro row %s", txs[0].SessionID, intro.ID)
	}
	if userBalanceSeconds(t, s.UserID) != 0 {
		t.Error("a free session must not move the balance")
	}

	// The transcript buffer belongs to the new row now.
	if got := s.getHistoryLog(); got != "" {
		t.Errorf("history was not reset at rollover: %q", got)
	}
}

// A paid session handed over from a check-in must be debited when its row closes:
// Cleanup only debits the row current at connection close, so the check-in's own
// minutes would otherwise never be charged.
func TestRolloverDebitsEndedPaidHalf(t *testing.T) {
	setupRolloverTestDB(t)
	// Put the user past the free allowance before the check-in opens. "Past the opening
	// pair" is now a count of the sessions on record rather than a users.state value, so
	// without these rows the movement half would legitimately still be free.
	for i, id := range []string{"prior-1", "prior-2"} {
		if err := database.DB.Exec(
			`INSERT INTO communication_sessions (id, user_id, start_time, duration) VALUES (?, ?, ?, ?)`,
			id, "user-1", time.Now().Add(-time.Duration(i+2)*time.Hour), 600).Error; err != nil {
			t.Fatalf("failed to seed prior session %s: %v", id, err)
		}
	}
	s := newRolloverTestSession(t, string(models.StateCheckin), api.SessionTypeCheckin, 3*time.Minute)
	checkinID := s.SessionDB.ID
	s.balanceExempt = false

	s.SessionType = api.SessionTypeSessionMovement
	s.rolloverSessionDB()

	var tx models.BalanceTransaction
	if err := database.DB.First(&tx, "session_id = ?", checkinID).Error; err != nil {
		t.Fatalf("no debit recorded for the ended check-in half: %v", err)
	}
	if tx.AmountSeconds >= 0 {
		t.Errorf("debit amount = %d, want negative", tx.AmountSeconds)
	}
	if s.balanceExempt {
		t.Error("balanceExempt must stay false past the opening pair")
	}
	var movement models.CommunicationSession
	if err := database.DB.First(&movement, "id = ?", s.SessionDB.ID).Error; err != nil {
		t.Fatalf("movement row missing: %v", err)
	}
	if movement.SessionType == nil || *movement.SessionType != string(api.SessionTypeSessionMovement) {
		t.Errorf("new row type = %v, want session_movement", movement.SessionType)
	}
}

// userBalanceSeconds reads the stored balance, which a free session must leave alone.
func userBalanceSeconds(t *testing.T, userID string) int64 {
	t.Helper()
	var u models.User
	if err := database.DB.Select("balance_seconds").First(&u, "id = ?", userID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	return u.BalanceSeconds
}
