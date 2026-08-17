package chat

import (
	"testing"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMemoryTestSession(t *testing.T, state string) *ChatSession {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created: the Postgres "timestamp with time zone" type does not scan on SQLite.
	if err := db.Exec(`CREATE TABLE user_memories (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL,
		content TEXT NOT NULL, created_at DATETIME)`).Error; err != nil {
		t.Fatalf("ddl failed: %v", err)
	}
	database.DB = db
	name := "Filipa Santos"
	return &ChatSession{
		logger: zap.NewNop(),
		UserID: "user-1",
		User:   &models.User{ID: "user-1", Name: &name, State: &state},
	}
}

// A second insight-category save mid-closing must NOT re-arm the early-complete guard.
// QA: the model filed an extra insight during the commitment step, the unconditional
// counter reset made the guard reject a fully finished closing, and Rumi re-asked the
// commitment and clarity questions ("Pensei que estavas a ouvir-me").
func TestRepeatInsightSaveDoesNotResetClosingGuard(t *testing.T) {
	s := newMemoryTestSession(t, string(models.StateVisionEmotionalClosing))

	if _, err := s.handleSaveMemory(map[string]interface{}{
		"category": "insight", "content": "Percebeste que precisas de limites.",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.closingInsightSaved || s.closingTurnsAfterInsight != 0 {
		t.Fatalf("first insight must arm the guard (saved=%v turns=%d)",
			s.closingInsightSaved, s.closingTurnsAfterInsight)
	}

	// Two spoken turns later the closing dialogue is complete...
	s.closingTurnsAfterInsight = 2

	// ...and a stray second insight must leave the counter alone.
	if _, err := s.handleSaveMemory(map[string]interface{}{
		"category": "insight", "content": "Decides bloquear tempo para ti.",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.closingTurnsAfterInsight != 2 {
		t.Errorf("repeat insight reset closingTurnsAfterInsight to %d; the guard would falsely reject complete_current_task", s.closingTurnsAfterInsight)
	}
}

// Memories are shown on the user's own memories screen — their own name as an opening
// vocative ("Filipa, decides...") reads as someone else talking about them. The persona
// forbids it; the guard makes it hold.
func TestStripLeadingVocative(t *testing.T) {
	s := newMemoryTestSession(t, string(models.StateVisionIdealLife))

	if _, err := s.handleSaveMemory(map[string]interface{}{
		"category": "insight", "content": "Filipa, decides bloquear tempo na agenda para caminhar.",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var saved models.UserMemory
	if err := database.DB.First(&saved).Error; err != nil {
		t.Fatalf("memory not saved: %v", err)
	}
	if saved.Content != "Decides bloquear tempo na agenda para caminhar." {
		t.Errorf("vocative not stripped, saved: %q", saved.Content)
	}

	// The name anywhere else — or someone else's name — is left untouched.
	for _, keep := range []string{
		"Valorizas o tempo com a tua filha Maria.",
		"Percebeste que a Filipa do passado adiava tudo.",
	} {
		if got := stripLeadingVocative(keep, s.User); got != keep {
			t.Errorf("stripLeadingVocative(%q) = %q, want unchanged", keep, got)
		}
	}
	if got := stripLeadingVocative("Filipa,", s.User); got != "Filipa," {
		t.Errorf("a content that is only the vocative must be left alone, got %q", got)
	}
	if got := stripLeadingVocative("anything", nil); got != "anything" {
		t.Errorf("nil user must be a no-op, got %q", got)
	}
}
