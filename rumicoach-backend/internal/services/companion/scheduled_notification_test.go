package companion

import (
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
)

func setupNotificationTestDB(t *testing.T) {
	t.Helper()
	setupJourneyTestDB(t)
	for _, ddl := range []string{
		`CREATE TABLE integrations (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider TEXT, external_id TEXT,
			status TEXT NOT NULL DEFAULT 'pending', link_code TEXT, link_code_expires_at DATETIME,
			reply_mode TEXT, last_inbound_at DATETIME, last_outbound_at DATETIME,
			daily_inbound_count INTEGER NOT NULL DEFAULT 0, daily_inbound_date DATETIME,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE channel_messages (
			id TEXT PRIMARY KEY, binding_id TEXT NOT NULL, user_id TEXT NOT NULL,
			provider TEXT, direction TEXT, provider_message_id TEXT, type TEXT,
			body TEXT, media_id TEXT, status TEXT, input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0, expires_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT, preferred_language TEXT,
			ideal_life_vision TEXT, focus_area TEXT, state TEXT, balance_seconds INTEGER DEFAULT 0)`,
		`CREATE TABLE channel_follow_ups (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, binding_id TEXT NOT NULL, kind TEXT,
			payload_hint TEXT, scheduled_at DATETIME, sent_at DATETIME, failed_at DATETIME,
			sent_text TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE balance_transactions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, type TEXT NOT NULL,
			amount_seconds INTEGER NOT NULL, balance_after INTEGER NOT NULL,
			session_id TEXT UNIQUE, session_type TEXT, product TEXT,
			reference_id TEXT UNIQUE, description TEXT, created_at DATETIME)`,
		`CREATE TABLE ai_usage_records (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, kind TEXT NOT NULL, model TEXT NOT NULL,
			ref_type TEXT, ref_id TEXT,
			input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0,
			input_text_tokens INTEGER DEFAULT 0, output_text_tokens INTEGER DEFAULT 0,
			input_audio_tokens INTEGER DEFAULT 0, output_audio_tokens INTEGER DEFAULT 0,
			input_video_tokens INTEGER DEFAULT 0, output_video_tokens INTEGER DEFAULT 0,
			stt_model TEXT, stt_input_tokens INTEGER DEFAULT 0, stt_output_tokens INTEGER DEFAULT 0, stt_total_tokens INTEGER DEFAULT 0,
			tts_model TEXT, tts_input_tokens INTEGER DEFAULT 0, tts_output_tokens INTEGER DEFAULT 0, tts_total_tokens INTEGER DEFAULT 0,
			cost_micros BIGINT, price_version TEXT,
			created_at DATETIME)`,
	} {
		if err := database.DB.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	database.DB.Exec(`INSERT INTO users (id, name, preferred_language) VALUES ('u1','Armando','pt-PT')`)
}

func addIntegration(t *testing.T, id string, lastInbound *time.Time) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO integrations (id, user_id, provider, external_id, status, reply_mode, last_inbound_at, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, "u1", "telegram", "123", models.IntegrationActive, models.ChannelReplyModeText, lastInbound, time.Now(),
	).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
}

// A user with a working chat must not also get a push for the same notification: the two
// would be the same nudge twice, in two registers. The dispatcher's "exactly one channel"
// rule is what guarantees that, and composing on chat has to keep it intact by reporting
// success — so this pins the contract the notification dispatcher relies on.
func TestComposeRefusesOutsideServiceWindow(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	// Last inbound long ago: outside the free-form window, so we cannot hold a
	// conversation and must let the dispatcher fall through to push.
	old := time.Now().Add(-72 * time.Hour)
	addIntegration(t, "i1", &old)
	database.DB.Exec(`UPDATE integrations SET provider = 'whatsapp' WHERE id = 'i1'`)

	err := s.ComposeScheduledNotification(t.Context(), "u1", "i1", "Your plan awaits", "Ready for the next step?")
	if err == nil {
		t.Error("outside the service window composition must fail so push can take over")
	}
}

// Two unprompted messages close together read as automation however well written. The
// companion's own post-session follow-up should own the moment; the notification waits.
func TestComposeLeavesRoomAfterARecentProactiveMessage(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()
	recent := time.Now().Add(-time.Hour)
	addIntegration(t, "i1", &recent)

	// The companion already reached out an hour ago.
	database.DB.Exec(
		`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, created_at) VALUES ('m1','i1','u1','telegram',?,'text','Hey, how did it go?','sent',?)`,
		models.ChannelMessageOutbound, time.Now().Add(-time.Hour))

	err := s.ComposeScheduledNotification(t.Context(), "u1", "i1", "Your plan awaits", "Ready for the next step?")
	if err == nil {
		t.Error("a notification must not stack on top of a recent proactive message")
	}

	// Once the quiet period has passed, it is free to go.
	database.DB.Exec(`UPDATE channel_messages SET created_at = ? WHERE id = 'm1'`,
		time.Now().Add(-proactiveQuietPeriod-time.Hour))
	// It will fail at the Gemini call in tests, but it must get PAST the quiet-period
	// gate — the error must no longer be about leaving room.
	err = s.ComposeScheduledNotification(t.Context(), "u1", "i1", "Your plan awaits", "Ready for the next step?")
	if err != nil && err.Error() == "a message was already sent recently; leaving room" {
		t.Error("the quiet period should have expired")
	}
}

func TestComposeRequiresALiveIntegration(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	if err := s.ComposeScheduledNotification(t.Context(), "u1", "missing", "T", "M"); err == nil {
		t.Error("an unknown integration must fail rather than silently drop the notification")
	}

	recent := time.Now().Add(-time.Hour)
	addIntegration(t, "i1", &recent)
	database.DB.Exec(`UPDATE integrations SET status = ? WHERE id = 'i1'`, models.IntegrationRevoked)
	if err := s.ComposeScheduledNotification(t.Context(), "u1", "i1", "T", "M"); err == nil {
		t.Error("a revoked integration must fail so delivery falls back to push")
	}
}

// The base directive is what stops proactive messages degrading into "hi, how are you?".
// These are the rules worth pinning: everything else can be reworded freely.
func TestProactiveBaseDirective(t *testing.T) {
	d := proactiveBaseDirective

	// It must forbid the empty-ping openings by name, not merely discourage them.
	for _, banned := range []string{"how are you?", "just checking in", "how's it going?"} {
		if !strings.Contains(strings.ToLower(d), banned) {
			t.Errorf("the directive should explicitly rule out %q as an opening", banned)
		}
	}
	// And require the message to be about something real.
	for _, want := range []string{"ONE specific thing", "could have been sent to any user"} {
		if !strings.Contains(d, want) {
			t.Errorf("the directive should demand specificity, missing %q", want)
		}
	}
	// Guilt is the fastest way to lose someone who already feels behind.
	if !strings.Contains(d, "never with guilt") {
		t.Error("the directive must rule out guilt when something has slipped")
	}
	// Repetition across days is what makes a companion feel automated. The rule must
	// point at BOTH sources: the conversation log and the replayed list of previous
	// unprompted messages (which survives the log being purged).
	if !strings.Contains(d, "Do NOT reuse the opening") {
		t.Error("the directive must forbid repeating the previous opening")
	}
	if !strings.Contains(d, "recent unprompted messages") {
		t.Error("the directive must point at the replayed proactive-message list")
	}
	// The machinery must stay invisible.
	if !strings.Contains(d, "Never mention notifications") {
		t.Error("the directive must keep reminders/schedules out of the message")
	}
}

// A hardcoded English fallback reaching a Portuguese user exposes the machinery — worse
// than the silence it replaces.
func TestProactiveFallbackIsLocalized(t *testing.T) {
	s := newToolService() // no localizer wired
	pt := "pt-PT"
	if got := s.proactiveFallback(&models.User{PreferredLanguage: &pt}); got == "" {
		t.Error("there must always be a fallback")
	}
	// Without a localizer we cannot translate, but the call must not panic on a user
	// with no language set either.
	if got := s.proactiveFallback(&models.User{}); got == "" {
		t.Error("a user with no language must still get a fallback")
	}
	if got := s.proactiveFallback(nil); got == "" {
		t.Error("a nil user must not panic or produce an empty message")
	}
}
