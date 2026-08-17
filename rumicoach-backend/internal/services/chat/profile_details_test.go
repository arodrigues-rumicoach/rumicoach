package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newProfileTestSession(t *testing.T) *ChatSession {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created: the Postgres "timestamp with time zone" types do not scan on SQLite.
	if err := db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY, country TEXT, date_of_birth DATE, gender TEXT,
		balance_seconds INTEGER NOT NULL DEFAULT 0, state TEXT)`).Error; err != nil {
		t.Fatalf("ddl failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (id, balance_seconds) VALUES ('user-1', 600)`).Error; err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	database.DB = db
	return &ChatSession{logger: zap.NewNop(), UserID: "user-1", User: &models.User{ID: "user-1"}}
}

// The intro collects the registration details one answer at a time, so the tool has to
// accept partial calls and tell the model what is still missing.
func TestSaveProfileDetailsAccumulates(t *testing.T) {
	s := newProfileTestSession(t)

	out, err := s.handleSaveProfileDetails(map[string]interface{}{"country_code": "pt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "date of birth") || !strings.Contains(out, "gender") {
		t.Errorf("result should name what is still missing, got: %s", out)
	}
	// Codes are normalised to uppercase so the app's country picker can match them.
	if s.User.Country == nil || *s.User.Country != "PT" {
		t.Errorf("country = %v, want PT", s.User.Country)
	}

	out, err = s.handleSaveProfileDetails(map[string]interface{}{
		"date_of_birth": "1990-05-03",
		"gender":        "Female",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "complete") {
		t.Errorf("result should report completion, got: %s", out)
	}
	if s.User.Gender == nil || *s.User.Gender != "female" {
		t.Errorf("gender = %v, want female (lowercased)", s.User.Gender)
	}

	// The values must actually be persisted, not just held in memory.
	var stored models.User
	if err := database.DB.Where("id = ?", "user-1").First(&stored).Error; err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if stored.Country == nil || *stored.Country != "PT" || stored.Gender == nil || *stored.Gender != "female" {
		t.Errorf("not persisted: country=%v gender=%v", stored.Country, stored.Gender)
	}
	if stored.DateOfBirth == nil || stored.DateOfBirth.Format("2006-01-02") != "1990-05-03" {
		t.Errorf("date_of_birth = %v, want 1990-05-03", stored.DateOfBirth)
	}
	if stored.NeedsProfileDetails() {
		t.Error("profile should be complete once all three are saved")
	}
}

// A bad value must come back as a specific, actionable error — silently dropping the
// answer would leave the model believing it saved something it did not.
func TestSaveProfileDetailsRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{"country name instead of code", map[string]interface{}{"country_code": "Portugal"}, "alpha-2"},
		{"unparseable date", map[string]interface{}{"date_of_birth": "3rd of May 1990"}, "YYYY-MM-DD"},
		{"future date", map[string]interface{}{"date_of_birth": time.Now().AddDate(1, 0, 0).Format("2006-01-02")}, "future"},
		{"unsupported gender value", map[string]interface{}{"gender": "other"}, "male"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newProfileTestSession(t)
			out, err := s.handleSaveProfileDetails(c.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, "error") || !strings.Contains(out, c.want) {
				t.Errorf("expected an actionable error mentioning %q, got: %s", c.want, out)
			}
			// Nothing bad should have been written.
			if !s.User.NeedsProfileDetails() {
				t.Error("an invalid value must not count as a saved detail")
			}
		})
	}
}

// The user may decline to state a gender. That must not block the rest: the intro
// accepts it and moves on, so the other two details still save.
func TestSaveProfileDetailsWithoutGender(t *testing.T) {
	s := newProfileTestSession(t)
	out, err := s.handleSaveProfileDetails(map[string]interface{}{
		"country_code":  "BR",
		"date_of_birth": "1985-11-20",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "error") {
		t.Errorf("omitting gender is allowed, got: %s", out)
	}
	if s.User.Country == nil || *s.User.Country != "BR" || s.User.DateOfBirth == nil {
		t.Error("country and date of birth should still be saved when gender is omitted")
	}
}
