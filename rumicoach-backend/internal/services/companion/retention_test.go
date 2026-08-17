package companion

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRetentionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	database.DB = db
	for _, ddl := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, chat_history_retention_days INTEGER NOT NULL DEFAULT 7)`,
		`CREATE TABLE channel_messages (
			id TEXT PRIMARY KEY, binding_id TEXT, user_id TEXT NOT NULL, provider TEXT,
			direction TEXT, provider_message_id TEXT, type TEXT, body TEXT, media_id TEXT,
			status TEXT, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
			expires_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	db.Exec(`INSERT INTO users (id, chat_history_retention_days) VALUES ('u1', 7)`)
	db.Exec(`INSERT INTO users (id, chat_history_retention_days) VALUES ('u2', 7)`)
	return db
}

func addMsg(t *testing.T, db *gorm.DB, id, userID string, ageDays int) {
	t.Helper()
	created := time.Now().AddDate(0, 0, -ageDays)
	if err := db.Exec(
		`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, expires_at, created_at)
		 VALUES (?, 'b', ?, 'telegram', 'inbound', 'text', 'hi', 'processed', ?, ?)`,
		id, userID, created.AddDate(0, 0, 7), created).Error; err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func expiryOf(t *testing.T, db *gorm.DB, id string) *time.Time {
	t.Helper()
	var msg models.ChannelMessage
	if err := db.First(&msg, "id = ?", id).Error; err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return msg.ExpiresAt
}

// The rewrite IS the feature. Every message carries its own expiry, materialized when it
// was stored, so changing only the setting would leave the whole existing conversation
// living by the old rule — and lowering the retention is exactly when somebody expects the
// change to bite at once and retroactively.
func TestSetChatHistoryRetentionRewritesExistingMessages(t *testing.T) {
	db := setupRetentionDB(t)
	addMsg(t, db, "m-old", "u1", 5)
	addMsg(t, db, "m-new", "u1", 1)
	addMsg(t, db, "other", "u2", 5)

	otherBefore := expiryOf(t, db, "other")

	if err := SetChatHistoryRetention(db, "u1", 3); err != nil {
		t.Fatalf("set retention: %v", err)
	}

	// Lowering to 3 days puts the 5-day-old message in the past: it goes on the next sweep.
	oldExp := expiryOf(t, db, "m-old")
	if oldExp == nil || !oldExp.Before(time.Now()) {
		t.Errorf("a message older than the new retention should already be due, got %v", oldExp)
	}
	// The 1-day-old one lives two more days, counted from when it was written.
	newExp := expiryOf(t, db, "m-new")
	if newExp == nil || !newExp.After(time.Now()) {
		t.Errorf("a recent message should still have time left, got %v", newExp)
	}

	var stored int
	db.Table("users").Select("chat_history_retention_days").Where("id = 'u1'").Scan(&stored)
	if stored != 3 {
		t.Errorf("setting = %d, want 3", stored)
	}

	// Another user's conversation is not ours to re-date.
	if got := expiryOf(t, db, "other"); got == nil || otherBefore == nil || !got.Equal(*otherBefore) {
		t.Errorf("another user's expiry changed: %v -> %v", otherBefore, got)
	}
}

// "Keep indefinitely" is represented as absence, not as a date far in the future — a
// far-future date silently comes due one day, and nothing would explain the deletion.
func TestSetChatHistoryRetentionToNeverClearsExpiries(t *testing.T) {
	db := setupRetentionDB(t)
	addMsg(t, db, "m1", "u1", 2)

	if err := SetChatHistoryRetention(db, "u1", 0); err != nil {
		t.Fatalf("set retention: %v", err)
	}
	if got := expiryOf(t, db, "m1"); got != nil {
		t.Errorf("expiry should be cleared for 'keep indefinitely', got %v", got)
	}

	// And turning it back on re-dates everything from each message's own creation.
	if err := SetChatHistoryRetention(db, "u1", 7); err != nil {
		t.Fatalf("set retention back: %v", err)
	}
	got := expiryOf(t, db, "m1")
	if got == nil {
		t.Fatal("expiry should be restored when retention is turned back on")
	}
	// Written 2 days ago with 7 days of retention: due in about 5.
	if d := time.Until(*got); d < 4*24*time.Hour || d > 6*24*time.Hour {
		t.Errorf("expiry %v is not ~5 days out (%v)", got, d)
	}
}

func TestSetChatHistoryRetentionRejectsUnofferedValues(t *testing.T) {
	db := setupRetentionDB(t)
	// 1 day is deliberately not offered: the coach replays 72 hours, so a shorter
	// retention leaves it losing the thread mid-exchange with nothing to explain why.
	for _, days := range []int{1, 2, 5, 365, -1} {
		if err := SetChatHistoryRetention(db, "u1", days); err == nil {
			t.Errorf("%d days should be rejected", days)
		}
	}
	// And a rejected value changes nothing.
	var stored int
	db.Table("users").Select("chat_history_retention_days").Where("id = 'u1'").Scan(&stored)
	if stored != 7 {
		t.Errorf("a rejected value altered the setting: %d", stored)
	}
}

func TestChatMessageExpiry(t *testing.T) {
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if got := models.ChatMessageExpiry(base, 0); got != nil {
		t.Errorf("0 days means keep indefinitely, got %v", got)
	}
	got := models.ChatMessageExpiry(base, 7)
	if got == nil || !got.Equal(base.AddDate(0, 0, 7)) {
		t.Errorf("expiry = %v, want %v", got, base.AddDate(0, 0, 7))
	}
}
