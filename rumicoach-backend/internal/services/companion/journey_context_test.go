package companion

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupJourneyTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE communication_sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_type TEXT,
			start_time DATETIME, end_time DATETIME, duration INTEGER,
			recap TEXT, recap_title TEXT, user_session_insight TEXT)`,
		`CREATE TABLE commitments (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, origin TEXT, title TEXT,
			type TEXT, days TEXT, date TEXT, done BOOLEAN NOT NULL DEFAULT 0,
			ended_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE commitment_completions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, commitment_id TEXT NOT NULL,
			date TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE user_memories (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL,
			content TEXT NOT NULL, created_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	database.DB = db
}

func addSession(t *testing.T, id, sessionType string, end time.Time, seconds int, recapTitle, recap string) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration, recap_title, recap) VALUES (?,?,?,?,?,?,?,?)`,
		id, "u1", sessionType, end.Add(-time.Duration(seconds)*time.Second), end, seconds, recapTitle, recap,
	).Error; err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

// The screenshot bug: the coach told a user with a full plan that it saw no session and
// no commitments. Whatever else changes, the journey block must carry the facts the user
// asks about, and must never be silently empty for a user who has a focus and history.
func TestJourneyContextCarriesTheFactsUsersAskAbout(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()

	focus := "Career"
	state := string(models.StateCheckin)
	user := &models.User{ID: "u1", FocusArea: &focus, State: &state}

	addSession(t, "s1", "session_vision", now.AddDate(0, 0, -10), 1500,
		"Clareza sobre a carreira", "Exploraste a vida que queres e percebeste que estavas à espera de permissão.")
	addSession(t, "s2", "session_movement", now.AddDate(0, 0, -1), 1500, "", "")

	got := buildJourneyContext(user, time.UTC, zap.NewNop())

	for _, want := range []string{"Current Focus", "Career", "Next session", "Recent sessions"} {
		if !strings.Contains(got, want) {
			t.Errorf("journey context missing %q:\n%s", want, got)
		}
	}
	// The recap is the whole point — it is what lets the coach reference the session.
	if !strings.Contains(got, "à espera de permissão") {
		t.Errorf("recap text should appear verbatim so the coach can reference it:\n%s", got)
	}
	if !strings.Contains(got, "Clareza sobre a carreira") {
		t.Errorf("recap title should appear:\n%s", got)
	}
	// Internal vocabulary must never reach a prompt the coach speaks from.
	for _, leak := range []string{"session_movement", "session_vision", "CHECKIN"} {
		if strings.Contains(got, leak) {
			t.Errorf("internal identifier %q leaked into the coach's context:\n%s", leak, got)
		}
	}
}

// A user with nothing yet must produce an explicit "nothing yet", not an empty block —
// the coach needs to know the difference between "no data" and "I wasn't told".
func TestJourneyContextForFreshUser(t *testing.T) {
	setupJourneyTestDB(t)
	state := string(models.StateVisionIdealLife)
	user := &models.User{ID: "u1", State: &state}

	got := buildJourneyContext(user, time.UTC, zap.NewNop())
	if !strings.Contains(got, "not chosen yet") {
		t.Errorf("a user without a focus area should be told so explicitly:\n%s", got)
	}
	if !strings.Contains(got, "none completed yet") {
		t.Errorf("a user without sessions should be told so explicitly:\n%s", got)
	}
	if strings.Contains(got, "Current streak") {
		t.Errorf("no streak line should appear for a user with no sessions:\n%s", got)
	}
}

// Abandoned starts must not appear as sessions the user "did", nor build a streak —
// referencing a session the user does not remember having is worse than saying nothing.
func TestJourneyContextIgnoresAbandonedSessions(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()
	state := string(models.StateCheckin)
	user := &models.User{ID: "u1", State: &state}

	addSession(t, "s1", "checkin", now.Add(-2*time.Hour), 20, "", "")

	got := buildJourneyContext(user, time.UTC, zap.NewNop())
	if !strings.Contains(got, "none completed yet") {
		t.Errorf("a 20-second connection must not count as a completed session:\n%s", got)
	}
}

func TestCurrentSessionStreak(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()

	// Sessions today, yesterday and the day before — a live 3-day streak.
	for i := 0; i < 3; i++ {
		addSession(t, string(rune('a'+i)), "checkin", now.AddDate(0, 0, -i), 600, "", "")
	}
	if got := currentSessionStreak("u1", time.UTC); got != 3 {
		t.Errorf("streak = %d, want 3", got)
	}

	// A gap breaks it: a session 5 days ago does not extend the run.
	addSession(t, "old", "checkin", now.AddDate(0, 0, -5), 600, "", "")
	if got := currentSessionStreak("u1", time.UTC); got != 3 {
		t.Errorf("streak across a gap = %d, want 3", got)
	}
}

// A streak that ends yesterday is still alive — today simply has not happened yet.
func TestStreakSurvivesUntilTodayEnds(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()
	for i := 1; i <= 2; i++ {
		addSession(t, string(rune('a'+i)), "checkin", now.AddDate(0, 0, -i), 600, "", "")
	}
	if got := currentSessionStreak("u1", time.UTC); got != 2 {
		t.Errorf("streak ending yesterday = %d, want 2 (today is not over)", got)
	}
}

// The accountability block is what lets the coach ask "how did the walk go today?"
// instead of a vague "how are things?". It must count only the days a habit was
// actually scheduled for — otherwise a Mon/Wed/Fri habit looks like it was missed
// four times every week.
func TestAccountabilityContextCountsScheduledDaysOnly(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	user := &models.User{ID: "u1"}

	isoToday := int(today.Weekday())
	if isoToday == 0 {
		isoToday = 7
	}
	// A habit scheduled ONLY for today, kept today.
	if err := database.DB.Exec(
		`INSERT INTO commitments (id, user_id, title, type, days) VALUES ('c1','u1','Morning walk','recurring',?)`,
		fmt.Sprintf("[%d]", isoToday)).Error; err != nil {
		t.Fatalf("insert commitment: %v", err)
	}
	if err := database.DB.Exec(
		`INSERT INTO commitment_completions (id, user_id, commitment_id, date) VALUES ('x1','u1','c1',?)`,
		today.Format("2006-01-02")).Error; err != nil {
		t.Fatalf("insert completion: %v", err)
	}

	got := buildAccountabilityContext(user, time.UTC, zap.NewNop())
	if !strings.Contains(got, "Morning walk") {
		t.Fatalf("habit missing from accountability block:\n%s", got)
	}
	if !strings.Contains(got, "kept 1 of 1 scheduled days") {
		t.Errorf("only scheduled days should be counted, got:\n%s", got)
	}
	if !strings.Contains(got, "DONE today") {
		t.Errorf("today's completion should be stated plainly, got:\n%s", got)
	}
}

// A habit due today and not yet done must say so — that is the follow-up opportunity.
func TestAccountabilityFlagsDueToday(t *testing.T) {
	setupJourneyTestDB(t)
	now := time.Now().UTC()
	isoToday := int(now.Weekday())
	if isoToday == 0 {
		isoToday = 7
	}
	database.DB.Exec(`INSERT INTO commitments (id, user_id, title, type, days) VALUES ('c1','u1','Journal','recurring',?)`,
		fmt.Sprintf("[%d]", isoToday))

	got := buildAccountabilityContext(&models.User{ID: "u1"}, time.UTC, zap.NewNop())
	if !strings.Contains(got, "due today, not done yet") {
		t.Errorf("an unfinished habit due today should be flagged, got:\n%s", got)
	}
	// No recurring commitments at all means no block, not an empty heading.
	setupJourneyTestDB(t)
	if got := buildAccountabilityContext(&models.User{ID: "u1"}, time.UTC, zap.NewNop()); got != "" {
		t.Errorf("no recurring commitments should produce no block, got:\n%s", got)
	}
}

// The companion's continuity is supposed to come from memories, so the ones older than
// the shared 30-day window must still reach it — otherwise the coach forgets who the
// user is after a month.
func TestFoundationalMemoriesSurviveTheThirtyDayWindow(t *testing.T) {
	setupJourneyTestDB(t)
	old := time.Now().AddDate(0, -3, 0)
	recent := time.Now().AddDate(0, 0, -2)
	database.DB.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m1','u1','identity','Product designer in Lisbon, father of two.',?)`, old)
	database.DB.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m2','u1','insight','A fleeting thought.',?)`, old)
	database.DB.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m3','u1','values','Recent enough to be in the shared block.',?)`, recent)

	got := buildFoundationalMemories(&models.User{ID: "u1"}, zap.NewNop())
	if !strings.Contains(got, "Product designer in Lisbon") {
		t.Errorf("older identity memory should survive:\n%s", got)
	}
	// Recent ones already appear in the shared 30-day block; repeating them wastes context.
	if strings.Contains(got, "Recent enough") {
		t.Errorf("memories inside the 30-day window must not be duplicated here:\n%s", got)
	}
	// Insights are moment-in-time, not lasting facts about who they are.
	if strings.Contains(got, "A fleeting thought") {
		t.Errorf("only lasting categories belong here:\n%s", got)
	}
}
