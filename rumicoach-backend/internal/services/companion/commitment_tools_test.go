package companion

import (
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

func newToolService() *Service {
	return &Service{logger: zap.NewNop()}
}

func addCommitmentRow(t *testing.T, id, title, kind, days string, endedAt *time.Time) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO commitments (id, user_id, origin, title, type, days, done, ended_at, created_at) VALUES (?,?,?,?,?,?,0,?,?)`,
		id, "u1", "manual", title, kind, days, endedAt, time.Now().AddDate(0, 0, -10),
	).Error; err != nil {
		t.Fatalf("insert commitment: %v", err)
	}
}

// Users say "the walk", not a UUID. Matching has to be forgiving — but acting on the
// WRONG commitment is worse than asking, so ambiguity must come back as a question.
func TestResolveCommitment(t *testing.T) {
	setupJourneyTestDB(t)
	addCommitmentRow(t, "c1", "Morning walk", "recurring", "[1,3,5]", nil)
	addCommitmentRow(t, "c2", "Evening walk", "recurring", "[2,4]", nil)
	addCommitmentRow(t, "c3", "Journal", "recurring", "[1,2,3,4,5]", nil)

	// Exact title wins even though "walk" also appears in another.
	got, err := resolveCommitment("u1", "morning walk")
	if err != nil || got.ID != "c1" {
		t.Errorf("exact (case-insensitive) match failed: %v %v", got, err)
	}
	// A unique fragment resolves.
	if got, err := resolveCommitment("u1", "Journal"); err != nil || got.ID != "c3" {
		t.Errorf("unique match failed: %v %v", got, err)
	}
	// An ambiguous fragment must NOT guess.
	_, err = resolveCommitment("u1", "walk")
	if err == nil {
		t.Fatal("an ambiguous title must not resolve to one commitment")
	}
	if !strings.Contains(err.Error(), "Morning walk") || !strings.Contains(err.Error(), "Evening walk") {
		t.Errorf("the ambiguity error should name the candidates so the coach can ask: %v", err)
	}
	// An unknown title lists what the user actually has, so the coach can recover.
	_, err = resolveCommitment("u1", "meditate")
	if err == nil || !strings.Contains(err.Error(), "Journal") {
		t.Errorf("unknown title should list the real commitments, got: %v", err)
	}
}

// A habit the user already finished must not be a candidate — logging against something
// they closed weeks ago is never what they meant.
func TestResolveCommitmentSkipsEnded(t *testing.T) {
	setupJourneyTestDB(t)
	ended := time.Now().AddDate(0, 0, -2)
	addCommitmentRow(t, "c1", "Journal", "recurring", "[1,2,3]", &ended)

	if _, err := resolveCommitment("u1", "Journal"); err == nil {
		t.Error("an ended commitment must not be resolvable")
	}

	// One with a FUTURE horizon is still running, so it must resolve.
	future := time.Now().AddDate(0, 0, 20)
	addCommitmentRow(t, "c2", "Morning walk", "recurring", "[1,3,5]", &future)
	if got, err := resolveCommitment("u1", "Morning walk"); err != nil || got.ID != "c2" {
		t.Errorf("a commitment with a future end is still live: %v %v", got, err)
	}
}

// Completion must use the same store the app uses, or the two disagree in front of the
// user: a per-day row for recurring habits, the master flag for one-time ones.
func TestCompleteCommitmentWritesWhereTheAppReads(t *testing.T) {
	setupJourneyTestDB(t)
	s := newToolService()
	addCommitmentRow(t, "c1", "Morning walk", "recurring", "[1,2,3,4,5,6,7]", nil)
	today := time.Now().UTC().Format("2006-01-02")

	res := s.completeCommitment("u1", map[string]any{"title": "Morning walk"})
	if res["status"] != "success" {
		t.Fatalf("completion failed: %v", res)
	}
	var count int64
	database.DB.Model(&models.CommitmentCompletion{}).
		Where("user_id = ? AND commitment_id = ? AND date = ?", "u1", "c1", today).Count(&count)
	if count != 1 {
		t.Errorf("recurring completion should create exactly one per-day row, got %d", count)
	}

	// Repeating it must not duplicate the row.
	s.completeCommitment("u1", map[string]any{"title": "Morning walk"})
	database.DB.Model(&models.CommitmentCompletion{}).
		Where("user_id = ? AND commitment_id = ? AND date = ?", "u1", "c1", today).Count(&count)
	if count != 1 {
		t.Errorf("completing twice should stay idempotent, got %d rows", count)
	}

	// Undo removes it.
	s.completeCommitment("u1", map[string]any{"title": "Morning walk", "done": false})
	database.DB.Model(&models.CommitmentCompletion{}).
		Where("user_id = ? AND commitment_id = ? AND date = ?", "u1", "c1", today).Count(&count)
	if count != 0 {
		t.Errorf("undo should remove the day's completion, got %d rows", count)
	}
}

func TestCompleteOneTimeUsesMasterFlag(t *testing.T) {
	setupJourneyTestDB(t)
	s := newToolService()
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, date, done, created_at) VALUES ('c1','u1','manual','Send CV','one_time','2026-08-05',0,?)`, time.Now())

	if res := s.completeCommitment("u1", map[string]any{"title": "Send CV"}); res["status"] != "success" {
		t.Fatalf("completion failed: %v", res)
	}
	var c models.Commitment
	database.DB.Where("id = ?", "c1").First(&c)
	if !c.Done {
		t.Error("a one-time commitment should be completed via its master flag")
	}
	// And no stray per-day row.
	var count int64
	database.DB.Model(&models.CommitmentCompletion{}).Where("commitment_id = ?", "c1").Count(&count)
	if count != 0 {
		t.Errorf("one-time completion must not write a per-day row, got %d", count)
	}
}

func TestAddCommitmentValidates(t *testing.T) {
	setupJourneyTestDB(t)
	s := newToolService()

	// A recurring habit with no weekdays cannot be scheduled — the coach must ask.
	res := s.addCommitment("u1", map[string]any{"title": "Stretch", "type": "recurring"})
	if res["status"] != "error" || !strings.Contains(res["message"].(string), "weekdays") {
		t.Errorf("recurring without days should ask for them, got %v", res)
	}
	// A one-time with no date likewise.
	res = s.addCommitment("u1", map[string]any{"title": "Dentist", "type": "one_time"})
	if res["status"] != "error" || !strings.Contains(res["message"].(string), "date") {
		t.Errorf("one_time without a date should ask for it, got %v", res)
	}
	// Nonsense type is refused rather than stored.
	if res := s.addCommitment("u1", map[string]any{"title": "X", "type": "weekly"}); res["status"] != "error" {
		t.Errorf("an unknown type must be refused, got %v", res)
	}

	// A valid recurring habit WITH a horizon.
	end := time.Now().AddDate(0, 0, 21).Format("2006-01-02")
	res = s.addCommitment("u1", map[string]any{
		"title": "Morning walk", "type": "recurring",
		"days": []any{float64(1), float64(3), float64(5)}, "end_date": end,
	})
	if res["status"] != "success" {
		t.Fatalf("valid add failed: %v", res)
	}
	var c models.Commitment
	database.DB.Where("title = ?", "Morning walk").First(&c)
	if len(c.Days) != 3 {
		t.Errorf("weekdays not stored: %v", c.Days)
	}
	if c.EndedAt == nil || c.EndedAt.Format("2006-01-02") != end {
		t.Errorf("horizon not stored: %v", c.EndedAt)
	}

	// Without a horizon it still saves, but the coach is told to go get one.
	res = s.addCommitment("u1", map[string]any{
		"title": "Drink water", "type": "recurring", "days": []any{float64(2)},
	})
	if res["status"] != "success" || !strings.Contains(res["message"].(string), "no end date") {
		t.Errorf("a habit without a horizon should prompt the coach to agree one, got %v", res)
	}
}

// Ending is finishing, not erasing: the row survives so the history does.
func TestEndCommitmentPreservesHistory(t *testing.T) {
	setupJourneyTestDB(t)
	s := newToolService()
	addCommitmentRow(t, "c1", "Journal", "recurring", "[1,2,3]", nil)
	database.DB.Exec(`INSERT INTO commitment_completions (id, user_id, commitment_id, date) VALUES ('x1','u1','c1','2026-07-20')`)

	if res := s.endCommitment("u1", map[string]any{"title": "Journal"}); res["status"] != "success" {
		t.Fatalf("end failed: %v", res)
	}
	var c models.Commitment
	if err := database.DB.Where("id = ?", "c1").First(&c).Error; err != nil {
		t.Fatal("ending must not delete the commitment — the history would go with it")
	}
	if c.EndedAt == nil {
		t.Error("ended_at should be set")
	}
	var completions int64
	database.DB.Model(&models.CommitmentCompletion{}).Where("commitment_id = ?", "c1").Count(&completions)
	if completions != 1 {
		t.Errorf("past completions must survive, got %d", completions)
	}

	// A one-time commitment cannot be "ended" — it simply passes.
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, date, done, created_at) VALUES ('c2','u1','manual','Send CV','one_time','2026-08-05',0,?)`, time.Now())
	if res := s.endCommitment("u1", map[string]any{"title": "Send CV"}); res["status"] != "error" {
		t.Errorf("ending a one-time commitment should be refused, got %v", res)
	}
}
