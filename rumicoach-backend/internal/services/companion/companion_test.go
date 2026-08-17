package companion

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMetaLocale(t *testing.T) {
	cases := map[string]string{
		"pt-PT": "pt_PT",
		"en-US": "en_US",
		"de-DE": "de_DE",
	}
	for in, want := range cases {
		lang := in
		if got := metaLocale(&lang); got != want {
			t.Errorf("metaLocale(%q) = %q, want %q", in, got, want)
		}
	}
	if got := metaLocale(nil); got != "en" {
		t.Errorf("metaLocale(nil) = %q, want en", got)
	}
	empty := ""
	if got := metaLocale(&empty); got != "en" {
		t.Errorf(`metaLocale("") = %q, want en`, got)
	}
}

// Retention is per message and materialized on write, so the sweep is a plain expiry
// check. What it replaced purged a whole conversation once it fell quiet, which meant the
// user who wrote every day never had anything deleted at all — the exact case retention
// exists for. That user is the first case here.
func TestPurgeExpiredMessages(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db
	// Hand-written DDL: GORM writes the model's Postgres column types verbatim and SQLite
	// will not scan `timestamp with time zone` back out.
	if err := db.Exec(`CREATE TABLE channel_messages (
		id TEXT PRIMARY KEY, binding_id TEXT NOT NULL, user_id TEXT NOT NULL,
		provider TEXT NOT NULL, direction TEXT NOT NULL, provider_message_id TEXT UNIQUE,
		type TEXT NOT NULL, body TEXT, media_id TEXT, status TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
		expires_at DATETIME, created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("failed to create channel_messages: %v", err)
	}
	if err := db.Exec(`CREATE TABLE channel_follow_ups (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, binding_id TEXT NOT NULL, kind TEXT,
		payload_hint TEXT, scheduled_at DATETIME, sent_at DATETIME, failed_at DATETIME, sent_text TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("failed to create channel_follow_ups: %v", err)
	}

	insert := func(id string, age time.Duration, expires *time.Time) {
		if err := db.Exec(
			`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, expires_at, created_at)
			 VALUES (?, 'b', 'u', 'whatsapp', 'inbound', 'text', 'hi', 'processed', ?, ?)`,
			id, expires, time.Now().Add(-age)).Error; err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(48 * time.Hour)

	// A daily talker: old messages, all still within an active conversation.
	insert("expired-1", 10*24*time.Hour, &past)
	insert("expired-2", 8*24*time.Hour, &past)
	insert("live", time.Hour, &future)
	// Kept indefinitely by the user's own choice.
	insert("never", 400*24*time.Hour, nil)

	svc := NewService(zap.NewNop(), nil)
	svc.purgeExpiredMessages()

	var remaining []string
	db.Model(&models.ChannelMessage{}).Order("id").Pluck("id", &remaining)
	if len(remaining) != 2 || remaining[0] != "live" || remaining[1] != "never" {
		t.Errorf("sweep kept %v, want [live never]", remaining)
	}
}

// Follow-ups are a work queue, not conversation. Only drained rows go, and only old ones:
// anything still pending is work nobody has done yet.
func TestPurgeLeavesPendingFollowUps(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db
	if err := db.Exec(`CREATE TABLE channel_messages (
		id TEXT PRIMARY KEY, binding_id TEXT, user_id TEXT, provider TEXT, direction TEXT,
		provider_message_id TEXT, type TEXT, body TEXT, media_id TEXT, status TEXT,
		input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
		expires_at DATETIME, created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	if err := db.Exec(`CREATE TABLE channel_follow_ups (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, binding_id TEXT NOT NULL, kind TEXT,
		payload_hint TEXT, scheduled_at DATETIME, sent_at DATETIME, failed_at DATETIME, sent_text TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}

	old := time.Now().AddDate(0, 0, -(followUpRetentionDays + 1))
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, sent_at, created_at) VALUES ('sent-old','u','b','daily_nudge',?,?,?)`, old, old, old)
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, failed_at, created_at) VALUES ('failed-old','u','b','daily_nudge',?,?,?)`, old, old, old)
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, created_at) VALUES ('pending-old','u','b','post_session',?,?)`, old, old)
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, sent_at, created_at) VALUES ('sent-recent','u','b','daily_nudge',?,?,?)`, time.Now(), time.Now(), time.Now())

	NewService(zap.NewNop(), nil).purgeExpiredMessages()

	var left []string
	db.Model(&models.ChannelFollowUp{}).Order("id").Pluck("id", &left)
	if len(left) != 2 || left[0] != "pending-old" || left[1] != "sent-recent" {
		t.Errorf("sweep kept %v, want [pending-old sent-recent]", left)
	}
}

func TestTTSVoiceForUser(t *testing.T) {
	female := "female"
	male := "male"
	custom := "despina"

	if got := ttsVoiceForUser(nil); got != "Gacrux" {
		t.Errorf("default voice = %q", got)
	}
	if got := ttsVoiceForUser(&models.User{CoachGender: &male}); got != "Charon" {
		t.Errorf("male default voice = %q", got)
	}
	if got := ttsVoiceForUser(&models.User{CoachGender: &female}); got != "Gacrux" {
		t.Errorf("female default voice = %q", got)
	}
	if got := ttsVoiceForUser(&models.User{CoachVoice: &custom, CoachGender: &male}); got != "Despina" {
		t.Errorf("explicit voice = %q", got)
	}
}
