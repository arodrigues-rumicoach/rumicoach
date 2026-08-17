package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Sunday 2026-07-26 in a fixed zone — the reference example: a recurring commitment on
// Tuesday must resolve to 2026-07-28, a one-time on Wednesday to 2026-07-29, so the
// Tuesday one sorts first.
var sunday = time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)

func strPtr(s string) *string { return &s }

func TestNextOccurrence(t *testing.T) {
	cases := []struct {
		name       string
		commitment models.Commitment
		want       string
	}{
		{"recurring Tuesday from Sunday", models.Commitment{Type: "recurring", Days: models.IntSlice{2}}, "2026-07-28"},
		{"recurring Wed+Fri from Sunday picks Wednesday", models.Commitment{Type: "recurring", Days: models.IntSlice{3, 5}}, "2026-07-29"},
		{"recurring today (Sunday) counts as today", models.Commitment{Type: "recurring", Days: models.IntSlice{7}}, "2026-07-26"},
		{"recurring every day is today", models.Commitment{Type: "recurring", Days: models.IntSlice{1, 2, 3, 4, 5, 6, 7}}, "2026-07-26"},
		{"one_time future keeps its date", models.Commitment{Type: "one_time", Date: strPtr("2026-07-29")}, "2026-07-29"},
		{"one_time overdue keeps its past date", models.Commitment{Type: "one_time", Date: strPtr("2026-07-20")}, "2026-07-20"},
		{"one_time without date has no occurrence", models.Commitment{Type: "one_time"}, ""},
		{"recurring without days has no occurrence", models.Commitment{Type: "recurring"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextOccurrence(c.commitment, sunday); got != c.want {
				t.Fatalf("nextOccurrence = %q, want %q", got, c.want)
			}
		})
	}
}

// setupCommitmentHistoryDB builds the two tables the history view reads.
func setupCommitmentHistoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE commitments (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, origin TEXT NOT NULL DEFAULT 'manual',
			title TEXT NOT NULL, type TEXT NOT NULL, days TEXT, date TEXT,
			done BOOLEAN NOT NULL DEFAULT 0, ended_at DATETIME,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE commitment_completions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, commitment_id TEXT NOT NULL,
			date TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	database.DB = db
	return db
}

func historyOf(t *testing.T, commitments []models.Commitment, now time.Time) []api.Commitment {
	t.Helper()
	srv := &Server{logger: zap.NewNop()}
	rec := httptest.NewRecorder()
	srv.writeCommitmentHistory(rec, "u1", commitments, now)
	var got []api.Commitment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode history: %v (body %s)", err, rec.Body.String())
	}
	return got
}

// The bug QA hit: history returned ONE row per recurring commitment, pointing at a
// future date, so the past — the whole point of a history screen — was invisible. It
// must now return one entry per scheduled day the commitment lived through, each marked
// completed or missed.
func TestCommitmentHistoryExpandsRecurringOccurrences(t *testing.T) {
	setupCommitmentHistoryDB(t)
	// A Wednesday, so the weekday maths is explicit rather than relative to "today".
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)

	created := now.AddDate(0, 0, -14) // two weeks of history
	c := models.Commitment{
		ID: "c1", UserID: "u1", Title: "Morning walk", Type: "recurring",
		Origin: models.CommitmentOriginPlan, Days: models.IntSlice{1, 3}, // Mon + Wed
		CreatedAt: created,
	}
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, days, created_at) VALUES (?,?,?,?,?,?,?)`,
		c.ID, c.UserID, c.Origin, c.Title, c.Type, "[1,3]", created)
	// Kept it on two of those days.
	for _, d := range []string{"2026-07-20", "2026-07-22"} {
		database.DB.Exec(`INSERT INTO commitment_completions (id, user_id, commitment_id, date) VALUES (?,?,?,?)`,
			"x"+d, "u1", "c1", d)
	}

	got := historyOf(t, []models.Commitment{c}, now)
	if len(got) < 4 {
		t.Fatalf("expected several past occurrences, got %d: %+v", len(got), got)
	}

	var completed, missed int
	seenDates := map[string]bool{}
	for _, e := range got {
		if e.OccurrenceDate == nil {
			t.Fatal("history entries must carry the occurrence date")
		}
		d := *e.OccurrenceDate
		if seenDates[d] {
			t.Errorf("duplicate occurrence for %s", d)
		}
		seenDates[d] = true

		// Only past days, only scheduled weekdays, never before it existed.
		if d > now.Format("2006-01-02") {
			t.Errorf("history must not contain future occurrences, got %s", d)
		}
		if d < created.Format("2006-01-02") {
			t.Errorf("occurrence %s predates the commitment", d)
		}
		parsed, _ := time.Parse("2006-01-02", d)
		if wd := parsed.Weekday(); wd != time.Monday && wd != time.Wednesday {
			t.Errorf("occurrence %s falls on %s, which is not scheduled", d, wd)
		}

		switch e.Status {
		case api.Completed:
			completed++
		case api.Missed:
			missed++
		default:
			t.Errorf("history status = %q, want completed or missed", e.Status)
		}
		// A past row carrying a future "next date" is what made the old response
		// impossible to render.
		if e.NextDate != nil {
			t.Error("history entries must not carry nextDate")
		}
	}
	if completed != 2 {
		t.Errorf("completed = %d, want 2 (the days with a recorded completion)", completed)
	}
	if missed == 0 {
		t.Error("the days the user should have walked but did not must show as missed")
	}

	// Newest first.
	for i := 1; i < len(got); i++ {
		if *got[i].OccurrenceDate > *got[i-1].OccurrenceDate {
			t.Errorf("history not ordered newest first at %d", i)
		}
	}
}

// An ended recurring commitment keeps the history it earned, but stops owing days.
func TestCommitmentHistoryStopsAtEnd(t *testing.T) {
	setupCommitmentHistoryDB(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	created := now.AddDate(0, 0, -21)
	ended := now.AddDate(0, 0, -7)

	c := models.Commitment{
		ID: "c1", UserID: "u1", Title: "Journal", Type: "recurring",
		Origin: models.CommitmentOriginManual, Days: models.IntSlice{1, 2, 3, 4, 5},
		CreatedAt: created, EndedAt: &ended,
	}
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, days, created_at, ended_at) VALUES (?,?,?,?,?,?,?,?)`,
		c.ID, c.UserID, c.Origin, c.Title, c.Type, "[1,2,3,4,5]", created, ended)

	got := historyOf(t, []models.Commitment{c}, now)
	if len(got) == 0 {
		t.Fatal("an ended commitment must keep its history")
	}
	endedStr := ended.Format("2006-01-02")
	for _, e := range got {
		if *e.OccurrenceDate >= endedStr {
			t.Errorf("occurrence %s is on or after the end date %s — an ended habit owes nothing", *e.OccurrenceDate, endedStr)
		}
		if e.EndedAt == nil {
			t.Error("history entries should carry endedAt so the app can label the habit as finished")
		}
	}
}

// One-time commitments are the simple case: a single occurrence, done or missed, and
// never before its date arrives.
func TestCommitmentHistoryOneTime(t *testing.T) {
	setupCommitmentHistoryDB(t)
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)

	past := "2026-07-20"
	future := "2026-08-05"
	doneOne := models.Commitment{ID: "a", UserID: "u1", Title: "Send CV", Type: "one_time",
		Origin: models.CommitmentOriginPlan, Date: &past, Done: true, CreatedAt: now.AddDate(0, 0, -20)}
	missedOne := models.Commitment{ID: "b", UserID: "u1", Title: "Call gym", Type: "one_time",
		Origin: models.CommitmentOriginManual, Date: &past, Done: false, CreatedAt: now.AddDate(0, 0, -20)}
	futureOne := models.Commitment{ID: "c", UserID: "u1", Title: "Dentist", Type: "one_time",
		Origin: models.CommitmentOriginManual, Date: &future, CreatedAt: now}

	got := historyOf(t, []models.Commitment{doneOne, missedOne, futureOne}, now)
	if len(got) != 2 {
		t.Fatalf("history entries = %d, want 2 (the future one is not history yet): %+v", len(got), got)
	}
	byID := map[string]api.Commitment{}
	for _, e := range got {
		byID[e.Id] = e
	}
	if byID["a"].Status != api.Completed {
		t.Errorf("a done one-time should be completed, got %q", byID["a"].Status)
	}
	if byID["b"].Status != api.Missed {
		t.Errorf("an undone past one-time should be missed, got %q", byID["b"].Status)
	}
	if _, ok := byID["c"]; ok {
		t.Error("a future one-time must not appear in history")
	}
}

// A horizon in the FUTURE means the habit is still running — it must stay in the road
// ahead. Treating any end date as "over" would make a commitment the coach just created
// with a 30-day horizon vanish from the list the moment it was saved.
func TestFutureEndDateKeepsCommitmentLive(t *testing.T) {
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC) // a Wednesday
	future := now.AddDate(0, 0, 30)
	past := now.AddDate(0, 0, -3)

	live := models.Commitment{Type: "recurring", Days: models.IntSlice{1, 3, 5}, EndedAt: &future}
	over := models.Commitment{Type: "recurring", Days: models.IntSlice{1, 3, 5}, EndedAt: &past}

	// nextOccurrence itself is horizon-agnostic; the list applies the end.
	if got := nextOccurrence(live, now); got == "" {
		t.Fatal("a live recurring commitment should still have a next occurrence")
	}
	nextLive := nextOccurrence(live, now)
	if nextLive > future.Format("2006-01-02") {
		t.Errorf("next occurrence %s is beyond the horizon %s", nextLive, future.Format("2006-01-02"))
	}
	// The ended one's next occurrence falls past its end, which is what removes it.
	nextOver := nextOccurrence(over, now)
	if nextOver <= past.Format("2006-01-02") {
		t.Errorf("an ended commitment should have no occurrence within its lifetime, got %s", nextOver)
	}
}
