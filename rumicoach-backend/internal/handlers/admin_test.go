package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/quote"
	"github.com/rumi/rumi-be/pkg/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupSeedTestDB builds the subset of tables the test-setup fixtures write to.
// Columns are hand-created (like balance_test.go): the models' Postgres types get
// TEXT affinity under SQLite AutoMigrate and break time.Time scans.
func setupSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE wheel_of_life_exercises (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT, data TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE user_memories (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL,
			content TEXT NOT NULL, created_at DATETIME
		)`,
		`CREATE TABLE commitments (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, origin TEXT NOT NULL DEFAULT 'manual',
			title TEXT NOT NULL, type TEXT NOT NULL, days TEXT, date TEXT,
			done BOOLEAN NOT NULL DEFAULT 0, ended_at DATETIME, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE communication_sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, start_time DATETIME NOT NULL,
			end_time DATETIME, duration INTEGER, language TEXT, session_type TEXT,
			input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0, input_text_tokens INTEGER DEFAULT 0,
			output_text_tokens INTEGER DEFAULT 0, input_audio_tokens INTEGER DEFAULT 0,
			output_audio_tokens INTEGER DEFAULT 0, input_video_tokens INTEGER DEFAULT 0,
			output_video_tokens INTEGER DEFAULT 0, deepgram_duration REAL DEFAULT 0,
			stt_service TEXT, transcript TEXT, ai_notes TEXT, ai_evaluation REAL,
			user_evaluation REAL, user_feedback TEXT, user_session_insight TEXT,
			session_summary TEXT, recap TEXT, recap_title TEXT
		)`,
		`CREATE TABLE user_app_opens (
			id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL,
			open_date DATE NOT NULL, created_at DATETIME
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}
	return db
}

// TestResetUserForTestSetupClearsRegistrationDetails pins the other half of that
// contract: the "welcome" (new_user) persona must look like an account that was just
// created, so the intro actually asks for country, date of birth, and gender instead of
// skipping the collection phase with values left over from an earlier run.
func TestResetUserForTestSetupClearsRegistrationDetails(t *testing.T) {
	dob := time.Date(1987, 11, 25, 0, 0, 0, 0, time.UTC)
	user := models.User{
		ID:                   "user-1",
		Country:              sptr("PT"),
		Gender:               sptr("male"),
		DateOfBirth:          &dob,
		IdealLifeVision:      sptr("an old vision"),
		FocusArea:            sptr("Career"),
		JourneyTheme:         sptr("sunset_beach"),
		JourneyQuoteCategory: sptr("growth"),
		LatestSessionHandle:  sptr("handle-1"),
		State:                sptr(string(models.StateCheckin)),
	}

	resetUserForTestSetup(&user)

	if !user.NeedsProfileDetails() {
		t.Error("registration details survived the reset; the intro would skip collecting them")
	}
	if user.Country != nil || user.Gender != nil || user.DateOfBirth != nil {
		t.Errorf("country/gender/date_of_birth not cleared: %v %v %v", user.Country, user.Gender, user.DateOfBirth)
	}
	if user.State == nil || *user.State != string(models.StateOnboardingIntro) {
		t.Errorf("state = %v, want ONBOARDING_INTRO", user.State)
	}
	for name, got := range map[string]*string{
		"ideal_life_vision":      user.IdealLifeVision,
		"focus_area":             user.FocusArea,
		"journey_theme":          user.JourneyTheme,
		"journey_quote_category": user.JourneyQuoteCategory,
		"latest_session_handle":  user.LatestSessionHandle,
	} {
		if got != nil {
			t.Errorf("%s = %q, want cleared", name, *got)
		}
	}
}

// accountOwnedTables is every table POST /admin/reset-account must leave empty. The point
// of the action is an account that reads as never used, so a table missed here is not a
// partial reset — it is a "new" account the journey, the balance or the companion can
// still see a previous life through.
var accountOwnedTables = []string{
	"ai_usage_records", "balance_transactions", "behavior_check_ins", "behavior_plans", "channel_follow_ups",
	"channel_messages", "channel_usage_records", "commitment_completions", "commitments",
	"communication_sessions", "daily_journeys", "eisenhower_matrix_exercises", "feedbacks",
	"feedback_attachments", "identity_reflections", "integrations", "notifications",
	"recommendations", "user_app_opens", "user_badges", "user_devices", "user_memories",
	"wheel_of_life_exercises",
}

// callResetAccount runs POST /admin/reset-account as userID. The route is admin-gated by
// the router (selectiveAuth in cmd/server/main.go), so the handler itself only ever sees
// an authenticated caller resetting their own account.
func callResetAccount(t *testing.T, server *Server, userID string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/admin/reset-account", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	server.ResetAccount(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// TestResetAccountEmptiesEveryTable is the whole contract of the admin reset in one test:
// the caller's account keeps nothing, and the account next to it keeps everything. It runs
// against the same fixture as the user-facing erasure tests — a user with sessions, a
// balance, a ledger, a live chat binding, badges and a registered device — because that is
// exactly the state QA needs to get out of.
func TestResetAccountEmptiesEveryTable(t *testing.T) {
	seedResetFixture(t)
	// The fixture is written for the /me/data scopes, so it leaves some tables empty and
	// others seeded for u1 alone. Both accounts get a row in every table here: an empty
	// table proves nothing about a delete, and a table only u1 owns cannot show scoping.
	database.DB.Exec(`INSERT INTO channel_usage_records (id, user_id, binding_id, direction, total_tokens) VALUES ('cu1','u1','i1','inbound',120), ('cu2','u2','i2','inbound',80)`)
	database.DB.Exec(`INSERT INTO ai_usage_records (id, user_id, kind, model, total_tokens) VALUES ('ai1','u1','message','m',120), ('ai2','u2','session','m',80)`)
	database.DB.Exec(`INSERT INTO recommendations (id, user_id) VALUES ('r1','u1'), ('r2','u2')`)
	database.DB.Exec(`INSERT INTO eisenhower_matrix_exercises (id, user_id) VALUES ('e1','u1'), ('e2','u2')`)
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data) VALUES ('w1','u1','[]'), ('w2','u2','[]')`)
	database.DB.Exec(`INSERT INTO feedbacks (id, user_id, category, description) VALUES ('fb1','u1','bug','...'), ('fb2','u2','bug','...')`)
	database.DB.Exec(`INSERT INTO feedback_attachments (id, feedback_id, user_id, object_path) VALUES ('fa1','fb1','u1','u1/1.png'), ('fa2','fb2','u2','u2/1.png')`)
	database.DB.Exec(`INSERT INTO commitments (id, user_id, title, type, done) VALUES ('c2','u2','Correr','recurring',0)`)
	database.DB.Exec(`INSERT INTO commitment_completions (id, user_id, commitment_id, date) VALUES ('cc2','u2','c2','2026-08-01')`)
	database.DB.Exec(`INSERT INTO behavior_plans (id, user_id, task_id) VALUES ('bp2','u2','c2')`)
	database.DB.Exec(`INSERT INTO behavior_check_ins (id, user_id, status) VALUES ('bc2','u2',?)`, models.BehaviorCheckInKept)
	database.DB.Exec(`INSERT INTO user_badges (id, user_id, badge_type, earned_at) VALUES ('b5','u2',?,?)`, string(api.FirstSession), time.Now())
	database.DB.Exec(`INSERT INTO user_devices (id, user_id, fcm_token, platform) VALUES ('d2','u2','tok-2','android')`)
	database.DB.Exec(`INSERT INTO user_app_opens (user_id, open_date, created_at) VALUES ('u2',?,?)`, time.Now(), time.Now())
	database.DB.Exec(`INSERT INTO daily_journeys (id, user_id, date, session_type, quote_id) VALUES ('dg_u2','u2',?,'checkin','q3')`, time.Now().Format("2006-01-02"))
	database.DB.Exec(`INSERT INTO notifications (id, user_id, title, message, delay_hours, scheduled_at) VALUES ('n_u2','u2','Olá','...',24,?)`, time.Now())

	callResetAccount(t, newResetServer(), "u1")

	for _, table := range accountOwnedTables {
		if n := countRows(t, table, "u1"); n != 0 {
			t.Errorf("%s: %d rows survived the reset", table, n)
		}
		if n := countRows(t, table, "u2"); n == 0 {
			t.Errorf("%s: the other account's rows were deleted too", table)
		}
	}
}

// TestResetAccountRestoresTheProfileRow covers what the reset writes back rather than what
// it deletes: the row must read as freshly provisioned — no progress, no balance, the
// onboarding details gone so the intro collects them again — while still being the same
// account, signed in, with its consents intact. The bystander's row must not move.
func TestResetAccountRestoresTheProfileRow(t *testing.T) {
	seedResetFixture(t)
	database.DB.Exec(`UPDATE users SET focus_area = 'Career', ideal_life_vision = 'an old vision',
		gender = 'female', country = 'PT', date_of_birth = ?, theme = 'forest',
		chat_history_retention_days = 30, journey_reset_at = ?, last_platform = 'ios',
		terms_and_conditions_accepted_at = ?, ai_accepted_at = ? WHERE id = 'u1'`,
		time.Date(1987, 11, 25, 0, 0, 0, 0, time.UTC), time.Now(), time.Now(), time.Now())

	callResetAccount(t, newResetServer(), "u1")

	var user models.User
	if err := database.DB.Where("id = ?", "u1").First(&user).Error; err != nil {
		t.Fatalf("read back user: %v", err)
	}

	// The state a new customer starts in, not the raw column default: the account must
	// read as a first run to everything that inspects the row rather than the session.
	if user.State == nil || *user.State != string(models.StateOnboardingIntro) {
		t.Errorf("state = %v, want ONBOARDING_INTRO", user.State)
	}
	if user.BalanceSeconds != 0 {
		t.Errorf("balance_seconds = %d, want 0", user.BalanceSeconds)
	}
	if balanceOf(t, "u2") != 1200 {
		t.Errorf("the other account's balance changed: %d, want 1200", balanceOf(t, "u2"))
	}
	if user.JourneyResetAt != nil {
		t.Error("journey_reset_at was stamped; a reset account has no history to fence off")
	}
	if user.ChatHistoryRetentionDays != 7 || user.Theme == nil || *user.Theme != "waterfall" {
		t.Errorf("column defaults not restored: retention=%d theme=%v", user.ChatHistoryRetentionDays, user.Theme)
	}
	if !user.NeedsProfileDetails() {
		t.Error("country/date of birth/gender survived; the onboarding intro would skip collecting them")
	}
	if user.IdealLifeVision != nil || user.FocusArea != nil {
		t.Errorf("coaching state survived: vision=%v focus=%v", user.IdealLifeVision, user.FocusArea)
	}

	// Still the same account: a reset that erased the identity would be a deletion.
	if user.Email == nil || *user.Email != "a@b.c" || user.Name == nil || *user.Name != "Armando" {
		t.Errorf("identity did not survive: email=%v name=%v", user.Email, user.Name)
	}
	if !user.IsActive || user.DeletedAt != nil {
		t.Errorf("account left inactive/deleted: is_active=%v deleted_at=%v", user.IsActive, user.DeletedAt)
	}
	if user.TermsAndConditionsAcceptedAt == nil || user.AIAcceptedAt == nil {
		t.Error("consent timestamps were cleared; they are given at signup, not in the app")
	}
}

// TestResetUserForFreshAccount pins the other half: the profile row must come back as
// regional.CreateLocalProfile writes it at signup. Everything the account learned about
// itself goes, the column defaults come back, and the fields that say WHICH account this
// is survive — losing those would make "reset" indistinguishable from "delete".
func TestResetUserForFreshAccount(t *testing.T) {
	dob := time.Date(1987, 11, 25, 0, 0, 0, 0, time.UTC)
	then := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	user := models.User{
		ID:                           "user-1",
		Email:                        sptr("qa@rumi.coach"),
		PhoneNumber:                  sptr("+351900000000"),
		Name:                         sptr("Laura"),
		PreferredLanguage:            sptr("pt-PT"),
		Region:                       sptr("eu"),
		DataRegion:                   sptr("eu"),
		IsActive:                     true,
		TermsAndConditionsAcceptedAt: &then,
		AIAcceptedAt:                 &then,
		DateOfBirth:                  &dob,
		Gender:                       sptr("female"),
		Country:                      sptr("PT"),
		CoachGender:                  sptr("female"),
		CoachVoice:                   sptr("aoede"),
		IdealLifeVision:              sptr("an old vision"),
		IdealLifeVisionSetAt:         &then,
		FocusArea:                    sptr("Career"),
		JourneyTheme:                 sptr("sunset_beach"),
		JourneyQuoteCategory:         sptr("growth"),
		LatestSessionHandle:          sptr("handle-1"),
		LatestSessionHandleAt:        &then,
		LastOnlineAt:                 &then,
		LastPlatform:                 sptr("ios"),
		JourneyResetAt:               &then,
		DeletedAt:                    &then,
		State:                        sptr(string(models.StateCheckin)),
		Theme:                        sptr("forest"),
		ChatHistoryRetentionDays:     30,
		BalanceSeconds:               4200,
	}

	resetUserForFreshAccount(&user)

	for name, got := range map[string]*string{
		"gender":                 user.Gender,
		"country":                user.Country,
		"coach_gender":           user.CoachGender,
		"coach_voice":            user.CoachVoice,
		"ideal_life_vision":      user.IdealLifeVision,
		"focus_area":             user.FocusArea,
		"journey_theme":          user.JourneyTheme,
		"journey_quote_category": user.JourneyQuoteCategory,
		"latest_session_handle":  user.LatestSessionHandle,
		"last_platform":          user.LastPlatform,
	} {
		if got != nil {
			t.Errorf("%s = %q, want cleared", name, *got)
		}
	}
	for name, got := range map[string]*time.Time{
		"date_of_birth":            user.DateOfBirth,
		"ideal_life_vision_set_at": user.IdealLifeVisionSetAt,
		"latest_session_handle_at": user.LatestSessionHandleAt,
		"last_online_at":           user.LastOnlineAt,
		"journey_reset_at":         user.JourneyResetAt,
		"deleted_at":               user.DeletedAt,
	} {
		if got != nil {
			t.Errorf("%s = %v, want cleared", name, *got)
		}
	}
	if !user.NeedsProfileDetails() {
		t.Error("registration details survived; the onboarding intro would skip collecting them")
	}

	// Column defaults, as a fresh row would carry them — except state, which is the state
	// a new customer starts in rather than the value provisioning leaves behind.
	if user.State == nil || *user.State != string(models.StateOnboardingIntro) {
		t.Errorf("state = %v, want ONBOARDING_INTRO", user.State)
	}
	if user.Theme == nil || *user.Theme != "waterfall" {
		t.Errorf("theme = %v, want waterfall", user.Theme)
	}
	if user.ChatHistoryRetentionDays != 7 {
		t.Errorf("chat_history_retention_days = %d, want 7", user.ChatHistoryRetentionDays)
	}
	if user.BalanceSeconds != 0 {
		t.Errorf("balance_seconds = %d, want 0", user.BalanceSeconds)
	}

	// Identity and consent: this is a reset, not a deletion.
	if user.ID != "user-1" || user.Email == nil || *user.Email != "qa@rumi.coach" {
		t.Errorf("identity did not survive: id=%q email=%v", user.ID, user.Email)
	}
	if user.PhoneNumber == nil || user.Name == nil || user.PreferredLanguage == nil ||
		user.Region == nil || user.DataRegion == nil || !user.IsActive {
		t.Error("account identity fields were cleared; reset must not deactivate the account")
	}
	if user.TermsAndConditionsAcceptedAt == nil || user.AIAcceptedAt == nil {
		t.Error("consent timestamps were cleared; they are given at signup, not in the app")
	}
}

// TestSeedRealUserProfile pins the contract QA depends on: after any post-onboarding
// persona, the frontend must find a fully furnished user — vision, focus area, AI
// theme/quote picks, a scored wheel, memories in every category the memories screen
// filters (with an insight newest), and commitments covering every render shape.
func TestSeedRealUserProfile(t *testing.T) {
	db := setupSeedTestDB(t)
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	user := models.User{ID: "user-1"}

	if err := seedRealUserProfile(db, &user, now, journeyAfterMovement); err != nil {
		t.Fatalf("seedRealUserProfile: %v", err)
	}

	// User row: everything /me and the Journey screen read.
	if user.State == nil || *user.State != string(models.StateCheckin) {
		t.Errorf("state = %v, want CHECKIN", user.State)
	}
	// A post-onboarding persona has the intro behind it, so the details the intro
	// collects must already be set — otherwise the journey would route QA back into
	// the intro instead of the scenario the persona is named for.
	if user.NeedsProfileDetails() {
		t.Error("post-onboarding persona is missing the registration details the intro collects")
	}
	for name, got := range map[string]*string{
		"ideal_life_vision":      user.IdealLifeVision,
		"focus_area":             user.FocusArea,
		"journey_quote_category": user.JourneyQuoteCategory,
		"journey_theme":          user.JourneyTheme,
	} {
		if got == nil || *got == "" {
			t.Errorf("%s is empty, want a seeded value", name)
		}
	}
	// The AI picks must be values the rest of the system accepts, or the growth
	// screen silently falls back instead of showing the seeded theme/quotes.
	if theme := models.NormalizeJourneyTheme(*user.JourneyTheme); theme == "" {
		t.Errorf("journey_theme %q is not a valid theme slug", *user.JourneyTheme)
	}
	if cat := quote.NormalizeCategory(*user.JourneyQuoteCategory); cat == "" {
		t.Errorf("journey_quote_category %q is not a valid quote category", *user.JourneyQuoteCategory)
	}

	// Wheel of life: fully scored, and the focus area must be one of its entries
	// (the synthesis screen resolves the priority area's score by name).
	var wheels []models.WheelOfLifeExercise
	if err := db.Where("user_id = ?", user.ID).Find(&wheels).Error; err != nil || len(wheels) != 1 {
		t.Fatalf("wheel exercises = %d (err %v), want 1", len(wheels), err)
	}
	var items []struct {
		Name         string  `json:"name"`
		CurrentScore float64 `json:"currentScore"`
	}
	if err := json.Unmarshal([]byte(wheels[0].Data), &items); err != nil {
		t.Fatalf("wheel data is not valid JSON: %v", err)
	}
	foundFocus := false
	for _, it := range items {
		if it.CurrentScore <= 0 {
			t.Errorf("wheel area %q has no score, want a completed wheel", it.Name)
		}
		if it.Name == *user.FocusArea {
			foundFocus = true
		}
	}
	if !foundFocus {
		t.Errorf("focus area %q is not one of the wheel areas", *user.FocusArea)
	}

	// Memories: one per category the app's CategoryFilter offers, and the newest
	// must be an insight (the Journey screen surfaces the latest insight).
	var memories []models.UserMemory
	if err := db.Where("user_id = ?", user.ID).Order("created_at desc").Find(&memories).Error; err != nil {
		t.Fatalf("fetch memories: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range memories {
		if m.Content == "" {
			t.Errorf("memory %q has empty content", m.Category)
		}
		// Only categories save_memory can write are valid. Seeding anything else
		// (a stale key from the app's filter list, say) shows the user a memory
		// type that does not exist in the product.
		if !api.MemoryCategory(m.Category).Valid() {
			t.Errorf("memory category %q is not in the api.MemoryCategory enum", m.Category)
		}
		seen[m.Category] = true
	}
	for _, cat := range []api.MemoryCategory{api.Identity, api.Values, api.Needs, api.Context, api.Obstacles, api.Insight} {
		if !seen[string(cat)] {
			t.Errorf("no seeded memory in category %q (the memories screen filters it)", cat)
		}
	}
	if len(memories) == 0 || memories[0].Category != "insight" {
		t.Errorf("newest memory category = %q, want insight", memories[0].Category)
	}

	// Commitments: the Journey and commitments screens must have one of every shape —
	// recurring, upcoming one-time, overdue one-time, and both origins.
	var commitments []models.Commitment
	if err := db.Where("user_id = ?", user.ID).Find(&commitments).Error; err != nil {
		t.Fatalf("fetch commitments: %v", err)
	}
	todayStr := now.Format("2006-01-02")
	var recurring, upcoming, overdue, manual, plan int
	for _, c := range commitments {
		switch c.Origin {
		case models.CommitmentOriginManual:
			manual++
		case models.CommitmentOriginPlan:
			plan++
		}
		switch c.Type {
		case "recurring":
			if len(c.Days) == 0 {
				t.Errorf("recurring commitment %q has no days", c.Title)
			}
			recurring++
		case "one_time":
			if c.Date == nil || *c.Date == "" {
				t.Errorf("one-time commitment %q has no date", c.Title)
				continue
			}
			if *c.Date < todayStr {
				overdue++
			} else {
				upcoming++
			}
		}
	}
	for name, n := range map[string]int{
		"recurring": recurring, "upcoming one-time": upcoming,
		"overdue one-time": overdue, "manual-origin": manual, "plan-origin": plan,
	} {
		if n == 0 {
			t.Errorf("no %s commitment seeded", name)
		}
	}
}

// TestSeedCommitmentsGrowWithJourney pins the rule QA asked for: a persona that has
// only finished onboarding already has commitments to show, and one that also did
// Movement has strictly more — every session leaves something behind.
func TestSeedCommitmentsGrowWithJourney(t *testing.T) {
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	todayStr := now.Format("2006-01-02")

	commitmentsAt := func(t *testing.T, stage int) []models.Commitment {
		t.Helper()
		db := setupSeedTestDB(t)
		user := models.User{ID: "user-1"}
		if err := seedRealUserProfile(db, &user, now, stage); err != nil {
			t.Fatalf("seedRealUserProfile(stage %d): %v", stage, err)
		}
		var commitments []models.Commitment
		if err := db.Where("user_id = ?", user.ID).Find(&commitments).Error; err != nil {
			t.Fatalf("fetch commitments: %v", err)
		}
		return commitments
	}

	afterOnboarding := commitmentsAt(t, journeyAfterOnboarding)
	afterMovement := commitmentsAt(t, journeyAfterMovement)

	// The first session must leave the user with something on the Journey screen —
	// an empty commitments list right after onboarding is the bug this guards.
	if len(afterOnboarding) < 1 {
		t.Fatalf("commitments after onboarding = %d, want at least 1", len(afterOnboarding))
	}
	if len(afterMovement) <= len(afterOnboarding) {
		t.Errorf("commitments after Movement = %d, want more than after onboarding (%d)",
			len(afterMovement), len(afterOnboarding))
	}

	// Even at the earliest stage both render shapes exist, so the Journey and
	// commitments screens are never exercised with only one kind of row.
	var recurring, oneTime int
	for _, c := range afterOnboarding {
		switch c.Type {
		case "recurring":
			recurring++
		case "one_time":
			oneTime++
		}
	}
	if recurring == 0 || oneTime == 0 {
		t.Errorf("after onboarding: recurring=%d one_time=%d, want at least one of each", recurring, oneTime)
	}

	// A user two hours past onboarding cannot have missed a deadline yet; the
	// overdue state belongs to the personas that have been around longer.
	for _, c := range afterOnboarding {
		if c.Type == "one_time" && c.Date != nil && *c.Date < todayStr {
			t.Errorf("commitment %q is overdue right after onboarding, which no real user could be", c.Title)
		}
	}
	overdue := 0
	for _, c := range afterMovement {
		if c.Type == "one_time" && c.Date != nil && *c.Date < todayStr {
			overdue++
		}
	}
	if overdue == 0 {
		t.Error("no overdue commitment after Movement; the overdue state would never render")
	}
}

// TestSeedCompletedSessionVision checks the Vision session carries the end-of-session
// artifacts the app re-renders from history, and that the persisted synthesis payload
// matches the seeded profile. Vision — not the short onboarding intro — is the session
// that produces them.
func TestSeedCompletedSessionVision(t *testing.T) {
	db := setupSeedTestDB(t)
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	user := models.User{ID: "user-1"}
	if err := seedRealUserProfile(db, &user, now, journeyAfterOnboarding); err != nil {
		t.Fatalf("seedRealUserProfile: %v", err)
	}

	end := now.Add(-2 * time.Hour)
	if err := seedCompletedSession(db, &user, api.SessionTypeSessionVision, "test-seed-vision", end); err != nil {
		t.Fatalf("seedCompletedSession: %v", err)
	}

	var sess models.CommunicationSession
	if err := db.Where("id = ?", "test-seed-vision").First(&sess).Error; err != nil {
		t.Fatalf("fetch session: %v", err)
	}

	// The journey only counts sessions that are typed, ended, and long
	// enough (>= 5 min) — otherwise the gates treat the persona as never having
	// done onboarding and reroute it back there.
	if sess.SessionType == nil || *sess.SessionType != string(api.SessionTypeSessionVision) {
		t.Errorf("session_type = %v, want session_vision", sess.SessionType)
	}
	if sess.EndTime == nil {
		t.Fatal("session has no end time; the journey gates would ignore it")
	}
	if sess.Duration < int(5*time.Minute.Seconds()) {
		t.Errorf("session duration = %vs, want >= 300s so it counts as done", sess.Duration)
	}

	for name, got := range map[string]*string{
		"transcript":           sess.Transcript,
		"ai_notes":             sess.AINotes,
		"user_feedback":        sess.UserFeedback,
		"user_session_insight": sess.UserSessionInsight,
		"session_summary":      sess.SessionSummary,
	} {
		if got == nil || *got == "" {
			t.Errorf("%s is empty, want a seeded value", name)
		}
	}

	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(*sess.SessionSummary), &summary); err != nil {
		t.Fatalf("session_summary is not valid JSON: %v", err)
	}
	if summary["vision"] != *user.IdealLifeVision {
		t.Errorf("summary vision does not match the seeded user vision")
	}
	area, ok := summary["priority_area"].(map[string]interface{})
	if !ok || area["name"] != *user.FocusArea {
		t.Errorf("summary priority_area = %v, want the seeded focus area %q", summary["priority_area"], *user.FocusArea)
	}
	if summary["key_insight"] != *sess.UserSessionInsight {
		t.Errorf("summary key_insight does not match the session insight")
	}
}

// TestSeedCompletedSessionNonOnboarding checks non-onboarding sessions stay lean
// (no synthesis payload) but still count toward the journey gates.
func TestSeedCompletedSessionNonOnboarding(t *testing.T) {
	db := setupSeedTestDB(t)
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	user := models.User{ID: "user-1"}

	if err := seedCompletedSession(db, &user, api.SessionTypeSessionMovement, "test-seed-movement", now.AddDate(0, 0, -2)); err != nil {
		t.Fatalf("seedCompletedSession: %v", err)
	}

	var sess models.CommunicationSession
	if err := db.Where("id = ?", "test-seed-movement").First(&sess).Error; err != nil {
		t.Fatalf("fetch session: %v", err)
	}
	if sess.SessionType == nil || *sess.SessionType != string(api.SessionTypeSessionMovement) {
		t.Errorf("session_type = %v, want session_movement", sess.SessionType)
	}
	if sess.SessionSummary != nil {
		t.Errorf("non-onboarding session should have no synthesis payload, got one")
	}
	if sess.EndTime == nil || sess.Duration < int(5*time.Minute.Seconds()) {
		t.Error("session must be ended and >= 5m to count toward the journey gates")
	}
}

// TestSeedAppOpens verifies the streak source rows land on the right days.
func TestSeedAppOpens(t *testing.T) {
	db := setupSeedTestDB(t)
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)

	if err := seedAppOpens(db, "user-1", now, 0, 1, 2); err != nil {
		t.Fatalf("seedAppOpens: %v", err)
	}

	var opens []models.UserAppOpen
	if err := db.Where("user_id = ?", "user-1").Order("open_date desc").Find(&opens).Error; err != nil {
		t.Fatalf("fetch app opens: %v", err)
	}
	if len(opens) != 3 {
		t.Fatalf("app opens = %d, want 3", len(opens))
	}
	for i, want := range []string{"2026-07-28", "2026-07-27", "2026-07-26"} {
		if got := opens[i].OpenDate.Format("2006-01-02"); got != want {
			t.Errorf("app open %d = %s, want %s", i, got, want)
		}
	}
}
