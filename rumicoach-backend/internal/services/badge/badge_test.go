package badge

import (
	"fmt"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBadgeTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created (like the other suites): the models' Postgres column types get TEXT
	// affinity under SQLite AutoMigrate and break time.Time scans.
	for _, ddl := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, ideal_life_vision TEXT, balance_seconds INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE user_badges (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, badge_type TEXT NOT NULL, earned_at DATETIME NOT NULL)`,
		`CREATE TABLE user_app_opens (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, open_date DATE NOT NULL, created_at DATETIME)`,
		`CREATE TABLE commitments (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, origin TEXT, title TEXT, type TEXT, days TEXT, date TEXT, done BOOLEAN NOT NULL DEFAULT 0, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE behavior_check_ins (id TEXT PRIMARY KEY, plan_id TEXT, user_id TEXT NOT NULL, status TEXT NOT NULL, note TEXT, created_at DATETIME)`,
		`CREATE TABLE user_memories (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL, content TEXT NOT NULL, created_at DATETIME)`,
		`CREATE TABLE communication_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_type TEXT, start_time DATETIME, end_time DATETIME, duration INTEGER)`,
		`CREATE TABLE integrations (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider TEXT, external_id TEXT, status TEXT NOT NULL DEFAULT 'pending', link_code TEXT, link_code_expires_at DATETIME, reply_mode TEXT, last_inbound_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE wheel_of_life_exercises (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT, data TEXT, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	if err := db.Exec(`INSERT INTO users (id) VALUES ('u1')`).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	database.DB = db
}

func badgeTypes(badges []models.UserBadge) map[string]bool {
	m := make(map[string]bool, len(badges))
	for _, b := range badges {
		m[b.BadgeType] = true
	}
	return m
}

func allBadgesFor(t *testing.T, userID string) map[string]bool {
	t.Helper()
	var rows []models.UserBadge
	if err := database.DB.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		t.Fatalf("fetch badges: %v", err)
	}
	return badgeTypes(rows)
}

// A brand-new user earns nothing — no false positives from empty tables.
func TestEvaluateAwardsNothingForFreshUser(t *testing.T) {
	setupBadgeTestDB(t)
	if awarded := EvaluateAndAward("u1", nil, zap.NewNop()); len(awarded) != 0 {
		t.Errorf("fresh user earned %v, want none", badgeTypes(awarded))
	}
}

func TestEvaluateAwardsAndPersists(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()

	// visionSet needs BOTH the vision and a mapped wheel.
	database.DB.Exec(`UPDATE users SET ideal_life_vision = 'a calm life' WHERE id = 'u1'`)
	if earned := allBadgesFor(t, "u1"); earned[string(api.VisionSet)] {
		t.Fatal("visionSet must not be awarded before the wheel exists")
	}
	EvaluateAndAward("u1", nil, zap.NewNop())
	if earned := allBadgesFor(t, "u1"); earned[string(api.VisionSet)] {
		t.Error("visionSet awarded with vision but no wheel")
	}
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w1','u1','[{"name":"Career","currentScore":4}]', ?)`, now.AddDate(0, 0, -40))

	// 3-day streak (a qualifying session on each of 3 consecutive days — merely opening
	// the app no longer counts), one kept commitment, one finished deep session (which
	// doubles as today's streak day), an active integration.
	for i := 1; i <= 2; i++ {
		end := now.AddDate(0, 0, -i)
		database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,'checkin',?,?,600)`,
			fmt.Sprintf("ck%d", i), "u1", end.Add(-10*time.Minute), end)
	}
	database.DB.Exec(`INSERT INTO commitments (id, user_id, title, type, done) VALUES ('c1','u1','walk','one_time',1)`)
	database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES ('s1','u1','session_movement',?,?,1500)`, now.Add(-time.Hour), now)
	database.DB.Exec(`INSERT INTO integrations (id, user_id, provider, status) VALUES ('i1','u1','whatsapp','active')`)

	awarded := EvaluateAndAward("u1", nil, zap.NewNop())
	got := badgeTypes(awarded)
	for _, want := range []api.BadgeType{api.VisionSet, api.FirstCommitment, api.ThreeDayStreak, api.FirstDeepSession, api.AlwaysWithYou} {
		if !got[string(want)] {
			t.Errorf("expected %s to be awarded, got %v", want, got)
		}
	}
	for _, not := range []api.BadgeType{api.SevenDayStreak, api.AllThemesExplored, api.WheelRemapped, api.AreaImproved} {
		if got[string(not)] {
			t.Errorf("%s awarded prematurely", not)
		}
	}

	// Idempotent: a second run must not duplicate rows.
	if again := EvaluateAndAward("u1", nil, zap.NewNop()); len(again) != 0 {
		t.Errorf("second evaluation re-awarded %v", badgeTypes(again))
	}
	var count int64
	database.DB.Model(&models.UserBadge{}).Where("user_id = ? AND badge_type = ?", "u1", string(api.FirstCommitment)).Count(&count)
	if count != 1 {
		t.Errorf("firstCommitment rows = %d, want exactly 1", count)
	}
}

// Badges are never revoked: conditions that stop holding leave the row untouched.
func TestBadgesAreNeverRevoked(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		end := now.AddDate(0, 0, -i)
		database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,'checkin',?,?,600)`,
			fmt.Sprintf("ck%d", i), "u1", end.Add(-10*time.Minute), end)
	}
	EvaluateAndAward("u1", nil, zap.NewNop())
	if !allBadgesFor(t, "u1")[string(api.ThreeDayStreak)] {
		t.Fatal("threeDayStreak should have been awarded")
	}

	// The streak "breaks" (rows deleted) — the badge must survive the next evaluation.
	database.DB.Exec(`DELETE FROM communication_sessions WHERE user_id = 'u1'`)
	EvaluateAndAward("u1", nil, zap.NewNop())
	if !allBadgesFor(t, "u1")[string(api.ThreeDayStreak)] {
		t.Error("threeDayStreak was revoked after the streak broke")
	}
}

// An abandoned start (shorter than the done threshold) must not extend a streak —
// otherwise connecting for a few seconds each day farms streak badges.
func TestAbandonedSessionsDoNotCount(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		end := now.AddDate(0, 0, -i)
		database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,'checkin',?,?,30)`,
			fmt.Sprintf("ck%d", i), "u1", end.Add(-30*time.Second), end)
	}
	got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if got[string(api.ThreeDayStreak)] {
		t.Error("30-second sessions must not build a streak")
	}
	if got[string(api.FirstSession)] {
		t.Error("a 30-second session must not count toward the session ladder")
	}
}

// The session ladder counts sessions, not days: several sessions in one day each count,
// while the streak still sees a single day.
func TestSessionLadder(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	// Sessions land a minute apart so the run stays inside a day or two — enough to prove
	// the ladder is not just counting streak days.
	seed := func(from, to int) {
		for i := from; i < to; i++ {
			end := now.Add(-time.Duration(i) * time.Minute)
			database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,'checkin',?,?,600)`,
				fmt.Sprintf("s%d", i), "u1", end.Add(-10*time.Minute), end)
		}
	}

	seed(0, 1)
	got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if !got[string(api.FirstSession)] {
		t.Error("one finished session should award firstSession")
	}
	if got[string(api.TwentySessions)] {
		t.Error("twentySessions awarded after 1 session")
	}

	seed(1, 19)
	if got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop())); got[string(api.TwentySessions)] {
		t.Error("twentySessions awarded at 19 sessions")
	}

	seed(19, 20)
	got = badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if !got[string(api.TwentySessions)] {
		t.Error("20 finished sessions should award twentySessions")
	}
	if got[string(api.HundredSessions)] {
		t.Error("hundredSessions awarded at 20 sessions")
	}

	seed(20, 100)
	if got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop())); !got[string(api.HundredSessions)] {
		t.Error("100 finished sessions should award hundredSessions")
	}
}

func TestWheelJourneyBadges(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()

	// First wheel 40 days ago, remapped today with Career up by 2 — both wheel badges.
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w1','u1','[{"name":"Career","currentScore":4},{"name":"Health","currentScore":7}]', ?)`, now.AddDate(0, 0, -40))
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w2','u1','[{"name":"Career","currentScore":6},{"name":"Health","currentScore":6}]', ?)`, now)

	got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if !got[string(api.WheelRemapped)] {
		t.Error("wheelRemapped should be awarded for a re-mapping 40 days later")
	}
	if !got[string(api.AreaImproved)] {
		t.Error("areaImproved should be awarded for Career 4 → 6")
	}
}

// Two wheels on the same day (the Vision session writes per-area updates) must not
// count as a re-mapping, and a +1 improvement is not enough.
func TestWheelBadgesNegativeCases(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w1','u1','[{"name":"Career","currentScore":4}]', ?)`, now.Add(-2*time.Hour))
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w2','u1','[{"name":"Career","currentScore":5}]', ?)`, now)

	got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if got[string(api.WheelRemapped)] {
		t.Error("two wheels hours apart must not count as a re-mapping")
	}
	if got[string(api.AreaImproved)] {
		t.Error("a +1 change must not count as an improved area")
	}
}

func TestAllThemesExplored(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	themes := []string{"session_movement", "session_values", "session_energy", "session_decisions", "session_beliefs", "session_identity", "session_acceptance", "session_priorities"}
	for i, theme := range themes[:len(themes)-1] {
		database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,?,?,?,1500)`,
			theme, "u1", theme, now.Add(-time.Duration(i+1)*time.Hour), now)
	}
	got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop()))
	if got[string(api.AllThemesExplored)] {
		t.Error("all but one theme must not award allThemesExplored")
	}
	if !got[string(api.FirstDeepSession)] {
		t.Error("firstDeepSession should be awarded")
	}

	// The last theme completes the set. An unfinished session must not count, so
	// insert it without end_time first and confirm nothing changes.
	database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, duration) VALUES ('open','u1','session_priorities',?,0)`, now)
	if got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop())); got[string(api.AllThemesExplored)] {
		t.Error("an unfinished session must not complete the theme set")
	}
	database.DB.Exec(`UPDATE communication_sessions SET end_time = ? WHERE id = 'open'`, now)
	if got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop())); !got[string(api.AllThemesExplored)] {
		t.Error("all themes finished should award allThemesExplored")
	}
}

// The opening pair must not count as a deep session — Vision is the doorway.
func TestOpeningPairIsNotDeep(t *testing.T) {
	setupBadgeTestDB(t)
	now := time.Now()
	for _, st := range []string{"onboarding", "session_vision"} {
		database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?,?,?,?,?,1500)`,
			st, "u1", st, now.Add(-time.Hour), now)
	}
	if got := badgeTypes(EvaluateAndAward("u1", nil, zap.NewNop())); got[string(api.FirstDeepSession)] {
		t.Error("onboarding/vision must not award firstDeepSession")
	}
}
