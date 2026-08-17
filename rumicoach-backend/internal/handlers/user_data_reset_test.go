package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/badge"
	"github.com/rumi/rumi-be/internal/services/journey"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Every table the delete handlers touch. They run inside one transaction, so a single
// missing table rolls the whole thing back and the assertions below would pass for the
// wrong reason — hence all of them, even the ones a given test never reads.
var resetTables = []string{
	// Every column of models.User, not only the ones the scope handlers update:
	// POST /admin/reset-account writes the profile row back with Save(), which names them
	// all, so a partial table would fail there instead of testing anything.
	`CREATE TABLE users (
		id TEXT PRIMARY KEY, email TEXT, phone_number TEXT, name TEXT,
		date_of_birth DATETIME, gender TEXT, coach_gender TEXT, country TEXT,
		region TEXT, data_region TEXT, preferred_language TEXT,
		last_online_at DATETIME, last_platform TEXT,
		is_active BOOLEAN DEFAULT 1,
		terms_and_conditions_accepted_at DATETIME, marketing_accepted_at DATETIME,
		ai_accepted_at DATETIME,
		ideal_life_vision TEXT, ideal_life_vision_set_at DATETIME, focus_area TEXT,
		top_values TEXT,
		state TEXT, theme TEXT DEFAULT 'waterfall',
		journey_theme TEXT, journey_quote_category TEXT, coach_voice TEXT,
		latest_session_handle TEXT, latest_session_handle_at DATETIME,
		chat_history_retention_days INTEGER NOT NULL DEFAULT 7,
		journey_reset_at DATETIME,
		balance_seconds INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME, deleted_at DATETIME)`,
	`CREATE TABLE communication_sessions (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_type TEXT,
		start_time DATETIME, end_time DATETIME, duration INTEGER,
		language TEXT, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0, deepgram_duration REAL DEFAULT 0,
		stt_service TEXT, transcript TEXT, ai_notes TEXT, ai_evaluation REAL,
		user_evaluation REAL, user_feedback TEXT, user_session_insight TEXT,
		session_summary TEXT, recap TEXT, recap_title TEXT)`,
	`CREATE TABLE balance_transactions (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, type TEXT NOT NULL,
		amount_seconds INTEGER NOT NULL, balance_after INTEGER NOT NULL,
		session_id TEXT, session_type TEXT, product TEXT, reference_id TEXT UNIQUE,
		description TEXT, created_at DATETIME)`,
	`CREATE TABLE integrations (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider TEXT, external_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending', link_code TEXT, link_code_expires_at DATETIME,
		reply_mode TEXT, last_inbound_at DATETIME, last_outbound_at DATETIME,
		daily_inbound_count INTEGER NOT NULL DEFAULT 0, daily_inbound_date DATETIME,
		created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE channel_messages (
		id TEXT PRIMARY KEY, binding_id TEXT NOT NULL, user_id TEXT NOT NULL,
		provider TEXT, direction TEXT, provider_message_id TEXT, type TEXT,
		body TEXT, status TEXT, expires_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE channel_follow_ups (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, binding_id TEXT NOT NULL,
		kind TEXT, payload_hint TEXT, scheduled_at DATETIME, sent_at DATETIME,
		failed_at DATETIME, sent_text TEXT, created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE user_memories (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT, content TEXT, created_at DATETIME)`,
	`CREATE TABLE user_badges (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, badge_type TEXT NOT NULL, earned_at DATETIME)`,
	`CREATE TABLE commitments (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT, type TEXT, done BOOLEAN NOT NULL DEFAULT 0, ended_at DATETIME, created_at DATETIME)`,
	`CREATE TABLE commitment_completions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, commitment_id TEXT, date TEXT)`,
	`CREATE TABLE behavior_plans (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, task_id TEXT)`,
	`CREATE TABLE behavior_check_ins (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, status TEXT)`,
	`CREATE TABLE identity_reflections (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT, learned_identity TEXT, what_it_gave TEXT, what_it_costs TEXT, who_becoming TEXT, qualities TEXT, evidence TEXT, created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE acceptance_reflections (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT, expected TEXT, reality TEXT, cannot_control TEXT, can_influence TEXT, choose_to_accept TEXT, where_i_act TEXT, next_step TEXT, created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE wheel_of_life_exercises (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, data TEXT, created_at DATETIME)`,
	`CREATE TABLE eisenhower_matrix_exercises (id TEXT PRIMARY KEY, user_id TEXT NOT NULL)`,
	`CREATE TABLE goals (id TEXT PRIMARY KEY, user_id TEXT NOT NULL)`,
	`CREATE TABLE daily_journeys (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, date TEXT NOT NULL, session_type TEXT, quote_id TEXT, created_at DATETIME)`,
	`CREATE TABLE notifications (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT,
		title TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
		delay_hours INTEGER NOT NULL DEFAULT 0, scheduled_at DATETIME,
		time_sensitive BOOLEAN NOT NULL DEFAULT 0, sent_at DATETIME, sent_via TEXT,
		failed_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
	`CREATE TABLE recommendations (id TEXT PRIMARY KEY, user_id TEXT NOT NULL)`,
	`CREATE TABLE channel_usage_records (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, binding_id TEXT, direction TEXT, total_tokens INTEGER DEFAULT 0, created_at DATETIME)`,
	`CREATE TABLE ai_usage_records (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, kind TEXT, model TEXT, ref_type TEXT, ref_id TEXT, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0, input_video_tokens INTEGER DEFAULT 0, output_video_tokens INTEGER DEFAULT 0, stt_model TEXT, stt_input_tokens INTEGER DEFAULT 0, stt_output_tokens INTEGER DEFAULT 0, stt_total_tokens INTEGER DEFAULT 0, tts_model TEXT, tts_input_tokens INTEGER DEFAULT 0, tts_output_tokens INTEGER DEFAULT 0, tts_total_tokens INTEGER DEFAULT 0, cost_micros BIGINT, price_version TEXT, created_at DATETIME)`,
	`CREATE TABLE user_app_opens (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, open_date DATE, created_at DATETIME)`,
	`CREATE TABLE user_devices (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, fcm_token TEXT, platform TEXT)`,
	`CREATE TABLE feedbacks (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT, description TEXT, platform TEXT, app_version TEXT, os_version TEXT, device_model TEXT, context TEXT, created_at DATETIME)`,
	`CREATE TABLE feedback_attachments (id TEXT PRIMARY KEY, feedback_id TEXT NOT NULL, user_id TEXT NOT NULL, object_path TEXT, content_type TEXT, size_bytes INTEGER NOT NULL DEFAULT 0, created_at DATETIME)`,
}

// seedResetFixture gives us a user who has lived a little: sessions behind them, a
// balance they paid for, a ledger that explains it, a live chat binding and a
// conversation on it. "u2" is a second user whose rows must survive any purge aimed at
// the first.
func seedResetFixture(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	for _, ddl := range resetTables {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v\n%s", err, ddl)
		}
	}
	database.DB = db

	db.Exec(`INSERT INTO users (id, email, name, state, balance_seconds) VALUES ('u1','a@b.c','Armando','CHECKIN', 3600)`)
	db.Exec(`INSERT INTO users (id, email, name, state, balance_seconds) VALUES ('u2','x@y.z','Outro','CHECKIN', 1200)`)

	now := time.Now()
	db.Exec(`INSERT INTO communication_sessions
		(id, user_id, session_type, start_time, end_time, duration, language, input_tokens, output_tokens, total_tokens, deepgram_duration, stt_service, transcript, recap, recap_title, ai_notes, ai_evaluation, user_feedback, user_session_insight, session_summary)
		VALUES ('s1','u1','session_vision',?,?,900,'pt-PT',1000,2000,3000,42.5,'deepgram','[USER] ...','Um recap','Título','QA notes',8.0,'correu bem','o meu insight','{"a":1}')`,
		now.AddDate(0, 0, -3), now.AddDate(0, 0, -3))
	db.Exec(`INSERT INTO communication_sessions
		(id, user_id, session_type, start_time, end_time, duration, language, input_tokens, output_tokens, total_tokens, transcript)
		VALUES ('s2','u1','session_movement',?,?,900,'pt-PT',500,600,1100,'[USER] ...')`,
		now.AddDate(0, 0, -1), now.AddDate(0, 0, -1))
	db.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration, transcript) VALUES ('s3','u2','session_vision',?,?,900,'[USER] ...')`, now, now)

	db.Exec(`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, product, created_at) VALUES ('t1','u1','top_up',5400,5400,'starter',?)`, now.AddDate(0, 0, -4))
	db.Exec(`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at) VALUES ('t2','u1','session_usage',-900,4500,'s1','session_vision',?)`, now.AddDate(0, 0, -3))
	db.Exec(`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at) VALUES ('t3','u1','session_usage',-900,3600,'s2','session_movement',?)`, now.AddDate(0, 0, -1))
	db.Exec(`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, created_at) VALUES ('t4','u2','top_up',1200,1200,?)`, now)

	db.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m1','u1','identity','...',?)`, now)
	db.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m2','u1','insight','...',?)`, now)
	db.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m3','u2','identity','...',?)`, now)
	db.Exec(`INSERT INTO identity_reflections (id, user_id, session_id, learned_identity, who_becoming, qualities, created_at) VALUES ('ir1','u1','s1','...','...','["Coragem"]',?)`, now)
	db.Exec(`INSERT INTO identity_reflections (id, user_id, session_id, learned_identity, who_becoming, qualities, created_at) VALUES ('ir2','u2','s3','...','...','["Abertura"]',?)`, now)
	db.Exec(`INSERT INTO acceptance_reflections (id, user_id, session_id, expected, reality, created_at) VALUES ('ar1','u1','s1','...','...',?)`, now)
	db.Exec(`INSERT INTO acceptance_reflections (id, user_id, session_id, expected, reality, created_at) VALUES ('ar2','u2','s3','...','...',?)`, now)

	// A live chat binding, reached out to an hour ago and three messages in today.
	hourAgo := now.Add(-time.Hour)
	db.Exec(`INSERT INTO integrations (id, user_id, provider, external_id, status, reply_mode, last_inbound_at, last_outbound_at, daily_inbound_count, daily_inbound_date, created_at)
		VALUES ('i1','u1','telegram','123',?,?,?,?,3,?,?)`,
		models.IntegrationActive, models.ChannelReplyModeText, hourAgo, hourAgo, now.UTC().Format("2006-01-02"), now)
	db.Exec(`INSERT INTO integrations (id, user_id, provider, external_id, status, reply_mode, created_at) VALUES ('i2','u2','telegram','456',?,?,?)`,
		models.IntegrationActive, models.ChannelReplyModeText, now)

	db.Exec(`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, created_at) VALUES ('cm1','i1','u1','telegram','inbound','text','olá','processed',?)`, hourAgo)
	db.Exec(`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, created_at) VALUES ('cm2','i1','u1','telegram','outbound','text','olá de volta','sent',?)`, hourAgo)
	db.Exec(`INSERT INTO channel_messages (id, binding_id, user_id, provider, direction, type, body, status, created_at) VALUES ('cm3','i2','u2','telegram','inbound','text','oi','processed',?)`, now)
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, created_at) VALUES ('f1','u1','i1','daily_nudge',?,?)`, now, now)
	db.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, created_at) VALUES ('f2','u2','i2','daily_nudge',?,?)`, now, now)

	// Commitments: one kept, one habit check-in kept — together they earn firstCommitment.
	db.Exec(`INSERT INTO commitments (id, user_id, title, type, done, created_at) VALUES ('c1','u1','Caminhar','recurring',1,?)`, now)
	db.Exec(`INSERT INTO commitment_completions (id, user_id, commitment_id, date) VALUES ('cc1','u1','c1','2026-08-01')`)
	db.Exec(`INSERT INTO behavior_plans (id, user_id, task_id) VALUES ('bp1','u1','c1')`)
	db.Exec(`INSERT INTO behavior_check_ins (id, user_id, status) VALUES ('bc1','u1',?)`, models.BehaviorCheckInKept)

	db.Exec(`INSERT INTO user_badges (id, user_id, badge_type, earned_at) VALUES ('b1','u1',?,?)`, string(api.FirstCommitment), now)
	db.Exec(`INSERT INTO user_badges (id, user_id, badge_type, earned_at) VALUES ('b2','u1',?,?)`, string(api.TenInsights), now)
	db.Exec(`INSERT INTO user_badges (id, user_id, badge_type, earned_at) VALUES ('b3','u1',?,?)`, string(api.FirstSession), now)
	db.Exec(`INSERT INTO user_badges (id, user_id, badge_type, earned_at) VALUES ('b4','u1',?,?)`, string(api.AlwaysWithYou), now)

	// Analytics that must outlive an erasure: app opens, past growth snapshots, the
	// registered device, and the delivery record of a notification that already went out.
	db.Exec(`INSERT INTO user_app_opens (user_id, open_date, created_at) VALUES ('u1',?,?)`, now.AddDate(0, 0, -5), now)
	db.Exec(`INSERT INTO user_app_opens (user_id, open_date, created_at) VALUES ('u1',?,?)`, now.AddDate(0, 0, -1), now)
	db.Exec(`INSERT INTO daily_journeys (id, user_id, date, session_type, quote_id, created_at) VALUES ('dg_old','u1',?,'checkin','q1',?)`,
		now.AddDate(0, 0, -2).Format("2006-01-02"), now)
	db.Exec(`INSERT INTO daily_journeys (id, user_id, date, session_type, quote_id, created_at) VALUES ('dg_today','u1',?,'session_movement','q2',?)`,
		now.Format("2006-01-02"), now)
	db.Exec(`INSERT INTO user_devices (id, user_id, fcm_token, platform) VALUES ('d1','u1','tok-1','ios')`)
	db.Exec(`INSERT INTO notifications (id, user_id, title, message, delay_hours, scheduled_at, sent_at, sent_via, created_at) VALUES ('n_sent','u1','Bom dia','A tua sessão espera-te',24,?,?,'whatsapp',?)`,
		now.AddDate(0, 0, -1), now.AddDate(0, 0, -1), now)
	db.Exec(`INSERT INTO notifications (id, user_id, title, message, delay_hours, scheduled_at, created_at) VALUES ('n_pending','u1','Amanhã','Como correu o exame?',48,?,?)`,
		now.AddDate(0, 0, 1), now)
}

func countRows(t *testing.T, table, userID string) int64 {
	t.Helper()
	var n int64
	if err := database.DB.Table(table).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func balanceOf(t *testing.T, userID string) int64 {
	t.Helper()
	var seconds int64
	if err := database.DB.Table("users").Select("balance_seconds").Where("id = ?", userID).Scan(&seconds).Error; err != nil {
		t.Fatalf("read balance: %v", err)
	}
	return seconds
}

func hasBadge(t *testing.T, userID string, badge api.BadgeType) bool {
	t.Helper()
	var n int64
	database.DB.Table("user_badges").Where("user_id = ? AND badge_type = ?", userID, string(badge)).Count(&n)
	return n > 0
}

// callScope invokes DELETE /me/data with the given ?scope= (empty means omitted).
func callScope(t *testing.T, server *Server, userID, scope string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", "/v1/me/data", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), userID))
	var params api.DeleteCurrentUserDataParams
	if scope != "" {
		params.Scope = &scope
	}
	w := httptest.NewRecorder()
	server.DeleteCurrentUserData(w, req, params)
	return w
}

func callScopeOK(t *testing.T, server *Server, userID, scope string) {
	t.Helper()
	if w := callScope(t, server, userID, scope); w.Code != http.StatusNoContent {
		t.Fatalf("scope %q: expected 204, got %d: %s", scope, w.Code, w.Body.String())
	}
}

// callAccountDeletion runs DELETE /me, posing as an EU data plane so the sign-in erasure
// (local on the auth plane, over the network elsewhere) stays out of the test.
func callAccountDeletion(t *testing.T, server *Server, userID string) {
	t.Helper()
	prev := config.AppConfig
	config.AppConfig = &config.Config{Environment: "test", RegionCode: "eu"}
	t.Cleanup(func() { config.AppConfig = prev })

	req := httptest.NewRequest("DELETE", "/v1/me", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	server.DeleteCurrentUser(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func newResetServer() *Server { return NewServer(zap.NewNop(), nil, nil, nil) }

// --- scope parsing -----------------------------------------------------------------

func TestParseDataScopes(t *testing.T) {
	// Omitted means "all", so callers written before this parameter keep working.
	for _, raw := range []string{"", "   "} {
		got, err := parseDataScopes(raw)
		if err != nil || len(got) != 1 || got[0] != scopeAll {
			t.Errorf("parseDataScopes(%q) = %v, %v; want [all], nil", raw, got, err)
		}
	}

	got, err := parseDataScopes("memories, chat")
	if err != nil || len(got) != 2 || got[0] != scopeMemories || got[1] != scopeChat {
		t.Errorf("comma-separated scopes = %v, %v", got, err)
	}

	// "all" absorbs whatever it is combined with.
	if got, err := parseDataScopes("memories,all"); err != nil || len(got) != 1 || got[0] != scopeAll {
		t.Errorf("all should absorb the rest, got %v, %v", got, err)
	}

	// But a typo is still caught, even next to "all" — silently erasing everything
	// because a value was misspelled is the worst possible reading.
	if _, err := parseDataScopes("all,memoires"); err == nil {
		t.Error("a misspelled scope must be rejected even alongside all")
	}
	if _, err := parseDataScopes("bogus"); err == nil {
		t.Error("unknown scope must be rejected")
	}
}

func TestDeleteUserDataRejectsUnknownScope(t *testing.T) {
	seedResetFixture(t)
	w := callScope(t, newResetServer(), "u1", "memories,bogus")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	// The offending value must be named, or the client cannot tell which one was wrong.
	if body := w.Body.String(); !strings.Contains(body, "bogus") {
		t.Errorf("the error should name the offending value, got: %s", body)
	}
	// And nothing may have been erased on the way to rejecting it.
	if n := countRows(t, "user_memories", "u1"); n != 2 {
		t.Errorf("a rejected request must not delete anything, %d memories left of 2", n)
	}
}

// --- scope: memories ---------------------------------------------------------------

func TestScopeMemoriesForgetsWithoutBreakingAnythingElse(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	callScopeOK(t, server, "u1", "memories")

	if n := countRows(t, "user_memories", "u1"); n != 0 {
		t.Errorf("memories = %d, want 0", n)
	}
	// The identity and acceptance reflections are the same kind of personal record and
	// go with them.
	if n := countRows(t, "identity_reflections", "u1"); n != 0 {
		t.Errorf("identity reflections = %d, want 0", n)
	}
	if n := countRows(t, "identity_reflections", "u2"); n != 1 {
		t.Errorf("another user's identity reflection was purged: %d, want 1", n)
	}
	if n := countRows(t, "acceptance_reflections", "u1"); n != 0 {
		t.Errorf("acceptance reflections = %d, want 0", n)
	}
	if n := countRows(t, "acceptance_reflections", "u2"); n != 1 {
		t.Errorf("another user's acceptance reflection was purged: %d, want 1", n)
	}
	// The badge earned from insight memories goes with them, or the grid asserts an
	// achievement next to a counter reading zero.
	if hasBadge(t, "u1", api.TenInsights) {
		t.Error("tenInsights should be dropped with the memories that earned it")
	}
	// Everything else is untouched — this scope is meant to be the harmless one.
	if hasBadge(t, "u1", api.FirstSession) != true || hasBadge(t, "u1", api.FirstCommitment) != true {
		t.Error("unrelated badges must survive scope=memories")
	}
	if n := countRows(t, "communication_sessions", "u1"); n != 2 {
		t.Errorf("sessions = %d, want 2 untouched", n)
	}
	if n := countRows(t, "commitments", "u1"); n != 1 {
		t.Errorf("commitments = %d, want 1 untouched", n)
	}
	if n := countRows(t, "channel_messages", "u1"); n != 2 {
		t.Errorf("chat = %d, want 2 untouched", n)
	}
	if n := countRows(t, "user_memories", "u2"); n != 1 {
		t.Errorf("another user's memories were purged: %d, want 1", n)
	}
}

// --- scope: chat -------------------------------------------------------------------

func TestScopeChatClearsTheConversationButStaysConnected(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	callScopeOK(t, server, "u1", "chat")

	if n := countRows(t, "channel_messages", "u1"); n != 0 {
		t.Errorf("channel_messages = %d, want 0", n)
	}
	if n := countRows(t, "channel_follow_ups", "u1"); n != 0 {
		t.Errorf("channel_follow_ups = %d, want 0", n)
	}

	// Clearing the conversation is not disconnecting the channel.
	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("the binding must survive: %v", err)
	}
	if integration.Status != models.IntegrationActive {
		t.Errorf("binding status = %q, want active", integration.Status)
	}

	// THE regression this scope exists to avoid: the quiet period used to be a COUNT over
	// the messages just deleted, so clearing your chat for privacy meant Rumi could message
	// you again on the spot. It now lives on the binding.
	if integration.MayReachOutAfter(6 * time.Hour) {
		t.Error("purging the chat must not unlock an immediate proactive message")
	}
	// Same for the daily cap, which was otherwise an abuse path: clear the chat, buy
	// yourself a fresh allowance.
	if integration.DailyInboundCount != 3 {
		t.Errorf("daily inbound count = %d, want 3 preserved", integration.DailyInboundCount)
	}

	// Another user's conversation is not ours to clear.
	if n := countRows(t, "channel_messages", "u2"); n != 1 {
		t.Errorf("another user's chat was purged: %d, want 1", n)
	}
	// And nothing outside the chat moved.
	if n := countRows(t, "user_memories", "u1"); n != 2 {
		t.Errorf("memories = %d, want 2 untouched", n)
	}
}

// --- scope: commitments ------------------------------------------------------------

func TestScopeCommitmentsLeavesNoDanglingHabitPlans(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	callScopeOK(t, server, "u1", "commitments")

	for _, table := range []string{"commitments", "commitment_completions", "behavior_plans", "behavior_check_ins"} {
		if n := countRows(t, table, "u1"); n != 0 {
			t.Errorf("%s = %d, want 0", table, n)
		}
	}
	if hasBadge(t, "u1", api.FirstCommitment) {
		t.Error("firstCommitment should be dropped with the commitments that earned it")
	}
	// Sessions and memories are a different category entirely.
	if n := countRows(t, "communication_sessions", "u1"); n != 2 {
		t.Errorf("sessions = %d, want 2 untouched", n)
	}
	if n := countRows(t, "user_memories", "u1"); n != 2 {
		t.Errorf("memories = %d, want 2 untouched", n)
	}
}

// behavior_check_ins is half of commitmentsKept. Dropping the badge while leaving that
// half behind just re-awards it the next time badges are evaluated — which happens at the
// end of every session, so the user would see it come back on its own.
func TestScopeCommitmentsBadgesDoNotComeBack(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	callScopeOK(t, server, "u1", "commitments")
	badge.EvaluateAndAward("u1", time.UTC, zap.NewNop())

	if hasBadge(t, "u1", api.FirstCommitment) {
		t.Error("firstCommitment was re-awarded: its inputs were not fully cleared")
	}
}

// --- scope: all --------------------------------------------------------------------

// Session rows carry the measurements the business runs on — duration, per-modality token
// counts, Deepgram seconds. Those are usage and cost analytics, not personal content, so
// the row survives and everything about the user inside it is stripped.
func TestScopeAllRedactsSessionsButKeepsTheirMetrics(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	callScopeOK(t, server, "u1", "")

	var sessions []models.CommunicationSession
	if err := database.DB.Where("user_id = ?", "u1").Order("start_time").Find(&sessions).Error; err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want both rows kept for analytics", len(sessions))
	}

	for _, s := range sessions {
		if s.Transcript != nil || s.Recap != nil || s.RecapTitle != nil ||
			s.UserEvaluation != nil || s.UserFeedback != nil ||
			s.UserSessionInsight != nil || s.SessionSummary != nil {
			t.Errorf("session %s still carries the user's own words: %+v", s.ID, s)
		}
		if s.Duration == 0 || s.SessionType == nil {
			t.Errorf("session %s lost its measurements", s.ID)
		}
	}
	// The measurements are the whole reason the rows stay. Token counts are no longer
	// among them — they moved to ai_usage_records, which no erasure touches either.
	if sessions[0].DeepgramDuration != 42.5 {
		t.Errorf("usage analytics were lost: %+v", sessions[0])
	}
	// The QA review grades the coach, not the user, so it stays with the metrics.
	if sessions[0].AINotes == nil || sessions[0].AIEvaluation == nil {
		t.Errorf("the coach's QA review should survive redaction: %+v", sessions[0])
	}

	// Another user's sessions are untouched, content included.
	var other models.CommunicationSession
	database.DB.First(&other, "id = 's3'")
	if other.Transcript == nil {
		t.Error("another user's transcript was redacted")
	}
}

// The line drawn today: measurements and delivery records are not coaching data, and
// erasing them bought the user nothing while costing every retention and cost figure the
// product is read by.
func TestScopeAllKeepsTheAnalytics(t *testing.T) {
	seedResetFixture(t)
	callScopeOK(t, newResetServer(), "u1", "all")

	// Nothing has read user_app_opens since the streak moved to session history — it is
	// purely the retention record, a user id and a date.
	if n := countRows(t, "user_app_opens", "u1"); n != 2 {
		t.Errorf("user_app_opens = %d, want 2 kept", n)
	}
	// A push token is the same class of thing as a chat binding: a channel, not content.
	if n := countRows(t, "user_devices", "u1"); n != 1 {
		t.Errorf("user_devices = %d, want the registered device kept", n)
	}

	// Only TODAY's growth snapshot goes — otherwise /journey reuses the stale
	// proposal and the reset looks like it never happened. History stays.
	var dates []string
	database.DB.Table("daily_journeys").Where("user_id = 'u1'").Pluck("date", &dates)
	if len(dates) != 1 || dates[0] == time.Now().Format("2006-01-02") {
		t.Errorf("daily_journeys = %v, want only the historical row kept", dates)
	}

	// A delivered notification keeps its telemetry and loses its copy.
	var sent models.Notification
	if err := database.DB.First(&sent, "id = 'n_sent'").Error; err != nil {
		t.Fatalf("the delivery record should survive: %v", err)
	}
	if sent.Title != "" || sent.Message != "" {
		t.Errorf("notification copy was written about the user and must go: %q / %q", sent.Title, sent.Message)
	}
	if sent.SentVia == nil || *sent.SentVia != "whatsapp" || sent.SentAt == nil {
		t.Errorf("delivery telemetry was lost: %+v", sent)
	}

	// A queued one is deleted outright — it must not fire after the erasure, least of all
	// with its text blanked.
	var pending int64
	database.DB.Table("notifications").Where("id = 'n_pending'").Count(&pending)
	if pending != 0 {
		t.Error("a pending notification must not survive the erasure that preceded it")
	}
}

// Session rows survive every scope, so the reset stamp is the ONLY thing that makes
// starting over real. Without it the user sees a "cleared" account that still reports a
// streak, still proposes the fifth deep session, and whose coach opens with "in your last
// session…" — worse than not resetting at all.
func TestScopeProgressRestartsTheJourneyWithoutDeletingSessions(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	// Before: two sessions on the record, and they count.
	if got := models.JourneyStartFor(database.DB, "u1"); got != nil {
		t.Fatalf("a user who never reset should have no cutoff, got %v", got)
	}
	current, _, totalDays, _, err := server.CalculateUserStreak("u1", time.UTC)
	if err != nil {
		t.Fatalf("streak: %v", err)
	}
	if totalDays == 0 {
		t.Fatal("the fixture should start with session days on the record")
	}
	_ = current

	callScopeOK(t, server, "u1", "progress")

	// The rows are still there — the user may still SEE that those sessions happened,
	// and the duration analytics are untouched.
	if n := countRows(t, "communication_sessions", "u1"); n != 2 {
		t.Errorf("sessions = %d, want both rows kept for analytics and history", n)
	}
	var s1 models.CommunicationSession
	database.DB.First(&s1, "id = 's1'")
	if s1.Duration != 900 {
		t.Errorf("measurements were lost: %+v", s1)
	}

	// But nothing counts any more.
	cutoff := models.JourneyStartFor(database.DB, "u1")
	if cutoff == nil {
		t.Fatal("the reset must stamp journey_reset_at; nothing else makes it real")
	}
	_, _, totalDaysAfter, _, err := server.CalculateUserStreak("u1", time.UTC)
	if err != nil {
		t.Fatalf("streak after reset: %v", err)
	}
	if totalDaysAfter != 0 {
		t.Errorf("streak days after reset = %d, want 0", totalDaysAfter)
	}

	// The journey starts over. The fixture had Vision and Movement done, so before the
	// reset the next proposal was further down the sequence; afterwards nothing counts and
	// it goes back to the first deep session.
	var user models.User
	if err := database.DB.First(&user, "id = 'u1'").Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if got := journey.ProposeSession(&user, time.UTC); got != api.SessionTypeSessionVision {
		t.Errorf("proposed session = %v, want the journey restarted at Vision", got)
	}
	// NextDeepSession reports "not applicable" precisely because the user is back inside
	// the opening pair — which is the restart, seen from the preview's side.
	if _, _, ok := journey.NextDeepSession(&user, time.UTC); ok {
		t.Error("after a reset the user is back in the opening pair, so there is no next-deep preview")
	}

	// A session recorded AFTER the reset counts again — the cutoff is a line, not a mute.
	database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES ('s4','u1','checkin',?,?,900)`,
		time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	_, _, totalDaysLater, _, _ := server.CalculateUserStreak("u1", time.UTC)
	if totalDaysLater != 1 {
		t.Errorf("a session after the reset should count, got %d days", totalDaysLater)
	}

	// And another user's journey is untouched.
	if got := models.JourneyStartFor(database.DB, "u2"); got != nil {
		t.Errorf("another user was reset too: %v", got)
	}
}

func TestScopeAllErasesTheChatLog(t *testing.T) {
	seedResetFixture(t)
	callScopeOK(t, newResetServer(), "u1", "all")

	if n := countRows(t, "channel_messages", "u1"); n != 0 {
		t.Errorf("channel_messages = %d, want 0 — the chat log used to outlive every erasure path", n)
	}
	if n := countRows(t, "channel_follow_ups", "u1"); n != 0 {
		t.Errorf("channel_follow_ups = %d, want 0", n)
	}
}

// The money is not coaching data. The ledger is the accounting record and stands on its
// own; deleting it while leaving balance_seconds standing kept the balance but destroyed
// the statement explaining it.
func TestDeleteUserDataKeepsTheLedgerAndTheBalance(t *testing.T) {
	seedResetFixture(t)
	callScopeOK(t, newResetServer(), "u1", "all")

	if n := countRows(t, "balance_transactions", "u1"); n != 3 {
		t.Fatalf("ledger rows = %d, want all 3 preserved", n)
	}
	if got := balanceOf(t, "u1"); got != 3600 {
		t.Errorf("balance_seconds = %d, want 3600 untouched — erasing data is not a refund", got)
	}

	// The statement must still reconstruct the balance.
	var lastBalanceAfter int64
	database.DB.Table("balance_transactions").Select("balance_after").
		Where("user_id = 'u1'").Order("created_at desc").Limit(1).Scan(&lastBalanceAfter)
	if lastBalanceAfter != balanceOf(t, "u1") {
		t.Errorf("ledger no longer explains the balance: last balance_after=%d, balance_seconds=%d",
			lastBalanceAfter, balanceOf(t, "u1"))
	}
}

// A chat binding is a communication channel, not coaching data. Revoking it on a data
// reset silently cut the user off from Rumi — a consequence they would only notice days
// later, and one no wording on the button ever warned them about.
func TestDeleteUserDataKeepsTheChatIntegration(t *testing.T) {
	seedResetFixture(t)
	callScopeOK(t, newResetServer(), "u1", "all")

	if n := countRows(t, "integrations", "u1"); n != 1 {
		t.Errorf("integrations = %d, want the binding left connected", n)
	}
}

// --- account deletion --------------------------------------------------------------

// Closing the account anonymizes the person; it does not erase what they were charged.
func TestDeleteAccountKeepsTheLedgerAndTheBalance(t *testing.T) {
	seedResetFixture(t)
	callAccountDeletion(t, newResetServer(), "u1")

	if n := countRows(t, "balance_transactions", "u1"); n != 3 {
		t.Errorf("ledger rows = %d, want all 3 preserved on account deletion", n)
	}
	if got := balanceOf(t, "u1"); got != 3600 {
		t.Errorf("balance_seconds = %d, want 3600 untouched", got)
	}

	var email string
	database.DB.Table("users").Select("email").Where("id = 'u1'").Scan(&email)
	if email != "u1@anonymized.com" {
		t.Errorf("account should still be anonymized, email = %q", email)
	}
}

// Account deletion is the opposite case for the binding: it must not outlive the account,
// or Rumi keeps a live channel to someone who asked to be forgotten.
func TestDeleteAccountRevokesTheChatIntegration(t *testing.T) {
	seedResetFixture(t)
	callAccountDeletion(t, newResetServer(), "u1")

	if n := countRows(t, "integrations", "u1"); n != 0 {
		t.Errorf("integrations = %d, want the binding revoked with the account", n)
	}
	if n := countRows(t, "integrations", "u2"); n != 1 {
		t.Errorf("another user's binding was revoked: %d left, want 1", n)
	}
}

// The two gaps: message bodies and session content used to survive account deletion
// outright — the stronger action erased less than the data reset did.
func TestDeleteAccountErasesChatAndSessionContent(t *testing.T) {
	seedResetFixture(t)
	callAccountDeletion(t, newResetServer(), "u1")

	if n := countRows(t, "channel_messages", "u1"); n != 0 {
		t.Errorf("channel_messages = %d, want 0", n)
	}
	if n := countRows(t, "channel_follow_ups", "u1"); n != 0 {
		t.Errorf("channel_follow_ups = %d, want 0", n)
	}

	var sessions []models.CommunicationSession
	database.DB.Where("user_id = ?", "u1").Find(&sessions)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want the analytics rows kept", len(sessions))
	}
	for _, s := range sessions {
		if s.Transcript != nil || s.Recap != nil || s.UserSessionInsight != nil {
			t.Errorf("session %s still carries the user's words after account deletion", s.ID)
		}
	}
}

// The emailed export answers 202 and does the work afterwards, so the endpoint's own
// contract is narrow: accept only what can actually be delivered, and never claim to have
// sent anything. What it must NOT do is 202 an account with nowhere to send the file —
// that is a promise nobody can keep, and the user would wait for an email forever.
func TestRequestUserDataExport(t *testing.T) {
	seedResetFixture(t)
	server := newResetServer()

	// With no mail service wired the request must be refused, not accepted into a void —
	// and above all must not panic in a detached goroutine, which would take the process
	// down rather than one response.
	req := httptest.NewRequest("POST", "/v1/me/data/export", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "u1"))
	w := httptest.NewRecorder()
	server.RequestUserDataExport(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no mail service should give 503, got %d: %s", w.Code, w.Body.String())
	}

	// With one wired, the request is accepted and the work happens afterwards.
	prevSvc := notification.GlobalNotificationService
	notification.GlobalNotificationService = &notification.NotificationService{}
	t.Cleanup(func() { notification.GlobalNotificationService = prevSvc })

	req = httptest.NewRequest("POST", "/v1/me/data/export", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "u1"))
	w = httptest.NewRecorder()
	server.RequestUserDataExport(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	// An account with no address cannot receive it.
	database.DB.Exec(`UPDATE users SET email = NULL WHERE id = 'u1'`)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/me/data/export", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "u1"))
	server.RequestUserDataExport(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("an account with no email must be refused, got %d", w.Code)
	}

	// Unknown user, and no user at all.
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/me/data/export", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "nobody"))
	server.RequestUserDataExport(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown user = %d, want 404", w.Code)
	}

	w = httptest.NewRecorder()
	server.RequestUserDataExport(w, httptest.NewRequest("POST", "/v1/me/data/export", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated = %d, want 401", w.Code)
	}
}

// Both routes must describe the same person. The emailed copy is built by the same
// function as the download precisely so they cannot drift, and this pins that.
func TestExportIsIdenticalForBothRoutes(t *testing.T) {
	seedResetFixture(t)

	built := buildUserDataExport(database.DB, "u1")
	if _, ok := built["user"]; !ok {
		t.Fatal("the export should carry the profile")
	}

	req := httptest.NewRequest("GET", "/v1/me/data", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "u1"))
	w := httptest.NewRecorder()
	newResetServer().ExportCurrentUserData(w, req)

	var downloaded map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &downloaded); err != nil {
		t.Fatalf("download did not produce JSON: %v", err)
	}
	viaBuilder, _ := json.Marshal(built)
	var normalized map[string]interface{}
	_ = json.Unmarshal(viaBuilder, &normalized)

	if len(downloaded) != len(normalized) {
		t.Errorf("the two routes disagree on which blocks exist: %d vs %d", len(downloaded), len(normalized))
	}
	for k := range normalized {
		if _, ok := downloaded[k]; !ok {
			t.Errorf("block %q is missing from the downloaded copy", k)
		}
	}
}
