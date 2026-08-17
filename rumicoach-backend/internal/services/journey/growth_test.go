package journey

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGrowthTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Created via raw SQL: AutoMigrate keeps the postgres "timestamp with time
	// zone" column type, which the sqlite driver cannot scan back into time.Time.
	if err := db.Exec(`CREATE TABLE communication_sessions (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_type TEXT,
		start_time DATETIME NOT NULL, end_time DATETIME, duration INTEGER)`).Error; err != nil {
		t.Fatalf("ddl failed: %v", err)
	}
	database.DB = db
}

func addSession(t *testing.T, userID, sessionType string, start time.Time, minutes int) {
	t.Helper()
	var end *time.Time
	var duration int
	if minutes > 0 {
		e := start.Add(time.Duration(minutes) * time.Minute)
		end = &e
		duration = minutes * 60
	}
	if err := database.DB.Exec(
		`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?, ?, ?, ?, ?, ?)`,
		userID+sessionType+start.String(), userID, sessionType, start, end, duration).Error; err != nil {
		t.Fatalf("insert session failed: %v", err)
	}
}

// checkinUser is a user whose opening pair is behind them: the intro collected the
// profile details and the Vision session wrote the ideal-life vision. Those two
// artifacts — not users.state — are what the opening-pair routing reads, so a fixture
// that only set the state would be proposed Vision forever.
func checkinUser(id string) *models.User {
	state := string(models.StateCheckin)
	u := openingPairUser(id)
	u.State = &state
	visionAt := time.Now().Add(-30 * 24 * time.Hour)
	u.IdealLifeVisionSetAt = &visionAt
	return u
}

// openingPairUser has done the intro (the profile details are in) but has not yet
// produced an ideal-life vision — the state a user is in between the two halves of the
// first meeting.
func openingPairUser(id string) *models.User {
	dob := time.Date(1990, 5, 3, 0, 0, 0, 0, time.UTC)
	gender, country := "male", "PT"
	return &models.User{ID: id, DateOfBirth: &dob, Gender: &gender, Country: &country}
}

// QA reset fence: a tester who completed deep sessions in a previous round and then
// reset to a fresh persona must NOT have those real sessions count — without the fence
// the roadmap proposed Energy right after Vision, with Movement and Values missing
// (QA). Seeded test-seed-* rows bypass the cutoff (personas seed backdated histories).
func TestJourneyResetFencesRealSessionsButNotSeeds(t *testing.T) {
	setupGrowthTest(t)
	now := time.Now()
	utc := time.UTC

	if err := database.DB.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, journey_reset_at DATETIME)`).Error; err != nil {
		t.Fatalf("users ddl failed: %v", err)
	}
	resetAt := now.Add(-time.Hour)
	if err := database.DB.Exec(`INSERT INTO users (id, journey_reset_at) VALUES ('u-reset', ?)`, resetAt).Error; err != nil {
		t.Fatalf("user insert failed: %v", err)
	}

	// Real sessions from the previous QA round — all before the reset stamp.
	addSession(t, "u-reset", string(api.SessionTypeOnboarding), now.AddDate(0, 0, -5), 3)
	addSession(t, "u-reset", string(api.SessionTypeSessionVision), now.AddDate(0, 0, -5), 20)
	addSession(t, "u-reset", string(api.SessionTypeSessionMovement), now.AddDate(0, 0, -4), 20)
	addSession(t, "u-reset", string(api.SessionTypeSessionValues), now.AddDate(0, 0, -3), 20)

	// The persona's seeded opening pair: backdated BEFORE the reset stamp, but carrying
	// the test-seed prefix, so it must still count.
	for _, s := range []struct {
		id, typ string
		mins    int
	}{
		{"test-seed-onboarding-u-reset", string(api.SessionTypeOnboarding), 3},
		{"test-seed-vision-u-reset", string(api.SessionTypeSessionVision), 20},
	} {
		start := now.Add(-2 * time.Hour)
		end := start.Add(time.Duration(s.mins) * time.Minute)
		if err := database.DB.Exec(
			`INSERT INTO communication_sessions (id, user_id, session_type, start_time, end_time, duration) VALUES (?, 'u-reset', ?, ?, ?, ?)`,
			s.id, s.typ, start, end, s.mins*60).Error; err != nil {
			t.Fatalf("seed insert failed: %v", err)
		}
	}

	user := checkinUser("u-reset")
	user.JourneyResetAt = &resetAt

	// The real Movement/Values are fenced off; the seeded opening pair counts — so the
	// journey is exactly "opening pair done": Movement next, available now (it unlocks
	// the day after Vision, which the seed places two hours ago... same calendar day
	// means tomorrow, so just assert the SESSION, which is the bug being pinned).
	next, _, ok := NextDeepSession(user, utc)
	if !ok || next != api.SessionTypeSessionMovement {
		t.Errorf("after reset: want movement next (real sessions fenced, seeds counted), got %s (ok=%v)", next, ok)
	}

	upcoming := UpcomingDeepSessions(user, utc)
	if len(upcoming) == 0 || upcoming[0].Session != api.SessionTypeSessionMovement {
		t.Fatalf("after reset: roadmap must start at movement, got %+v", upcoming)
	}

	// Without the stamp the old sessions count again — guards that the fixture above
	// genuinely relies on the fence rather than passing by accident.
	user.JourneyResetAt = nil
	database.DB.Exec(`UPDATE users SET journey_reset_at = NULL WHERE id = 'u-reset'`)
	next, _, ok = NextDeepSession(user, utc)
	if !ok || next != api.SessionTypeSessionEnergy {
		t.Errorf("without reset: expected energy next (real movement+values count), got %s (ok=%v)", next, ok)
	}
}

func TestJourneyGating(t *testing.T) {
	setupGrowthTest(t)
	now := time.Now()

	utc := time.UTC

	// Mid-intro: the details it collects are still missing, so it is unfinished — no
	// next-session preview, because the opening pair IS today's session.
	introUser := &models.User{ID: "u-intro"}
	if _, _, ok := NextDeepSession(introUser, utc); ok {
		t.Error("mid-intro user must have no next-session preview")
	}
	if got := ProposeSession(introUser, utc); got != api.SessionTypeSessionVision {
		t.Errorf("mid-intro user must be proposed session_vision, got %s", got)
	}

	// Mid-Vision: details collected, vision not written yet. Still inside the opening
	// pair, so no preview — and the session to resume is Vision, not the intro they
	// already sat through.
	obUser := openingPairUser("u-ob")
	if _, _, ok := NextDeepSession(obUser, utc); ok {
		t.Error("mid-vision user must have no next-session preview")
	}
	if got := ProposeSession(obUser, utc); got != api.SessionTypeSessionVision {
		t.Errorf("mid-vision user must be proposed vision, got %s", got)
	}

	// The intro is short by design (a greeting, privacy, a roadmap and one question).
	// It must still register as done, or the user is sent back through it forever.
	shortIntro := openingPairUser("u-short")
	addSession(t, "u-short", string(api.SessionTypeOnboarding), now.Add(-2*time.Hour), 2)
	if got := ProposeSession(shortIntro, utc); got != api.SessionTypeSessionVision {
		t.Errorf("a completed 2-minute intro must count as done and lead to vision, got %s", got)
	}

	// A genuinely abandoned intro (a few seconds) still must not count.
	abandoned := openingPairUser("u-abandoned")
	addSession(t, "u-abandoned", string(api.SessionTypeOnboarding), now.Add(-2*time.Hour), 0)
	if got := ProposeSession(abandoned, utc); got != api.SessionTypeSessionVision {
		t.Errorf("an abandoned intro should skip to vision, got %s", got)
	}

	// Intro done, Vision not yet: Vision unlocks immediately — no overnight wait
	// between the two halves of one first meeting.
	userFresh := openingPairUser("u-5")
	freshIntro := startOfDay(now, utc).Add(time.Hour)
	addSession(t, "u-5", string(api.SessionTypeOnboarding), freshIntro, 3)
	if got := ProposeSession(userFresh, utc); got != api.SessionTypeSessionVision {
		t.Errorf("after the intro: want vision proposed immediately, got %s", got)
	}
	// Vision is part of the opening pair, so it is never previewed as a future chapter.
	if _, _, ok := NextDeepSession(userFresh, utc); ok {
		t.Error("vision must not be offered as a next-session preview")
	}

	// Vision done today: Movement unlocks at the start of the NEXT calendar day —
	// the hour Vision happened never matters.
	freshVision := startOfDay(now, utc).Add(2 * time.Hour)
	addSession(t, "u-5", string(api.SessionTypeSessionVision), freshVision, 25)
	// Doing the session is what writes the vision, and writing it is what closes the
	// opening pair — without this the user is still owed Vision, not offered Movement.
	userFresh.IdealLifeVisionSetAt = &freshVision
	next, availableAt, ok := NextDeepSession(userFresh, utc)
	wantAt := startOfDay(freshVision, utc).AddDate(0, 0, 1)
	if !ok || next != api.SessionTypeSessionMovement || !availableAt.Equal(wantAt) {
		t.Errorf("fresh vision: want movement at next midnight %s, got %s at %s (ok=%v)", wantAt, next, availableAt, ok)
	}
	if got := ProposeSession(userFresh, utc); got != api.SessionTypeCheckin {
		t.Errorf("fresh vision: want checkin proposed while movement is gated, got %s", got)
	}

	// Opening pair done days ago: Movement is open.
	user := checkinUser("u-1")
	addSession(t, "u-1", string(api.SessionTypeOnboarding), now.Add(-11*24*time.Hour), 3)
	addSession(t, "u-1", string(api.SessionTypeSessionVision), now.Add(-10*24*time.Hour), 25)
	next, availableAt, ok = NextDeepSession(user, utc)
	if !ok || next != api.SessionTypeSessionMovement || availableAt.After(now) {
		t.Errorf("after opening pair: want movement available, got %s at %s (ok=%v)", next, availableAt, ok)
	}
	if got := ProposeSession(user, utc); got != api.SessionTypeSessionMovement {
		t.Errorf("after opening pair: want movement proposed, got %s", got)
	}

	// Movement done 3 days ago: Values is next but gated until 5 calendar days
	// after movement's day; nothing deep proposed today (the app offers the
	// check-in instead).
	movementStart := now.Add(-3 * 24 * time.Hour)
	addSession(t, "u-1", string(api.SessionTypeSessionMovement), movementStart, 20)
	next, availableAt, ok = NextDeepSession(user, utc)
	wantAt = startOfDay(movementStart, utc).AddDate(0, 0, 5)
	if !ok || next != api.SessionTypeSessionValues || !availableAt.Equal(wantAt) {
		t.Errorf("gated: want values at %s, got %s at %s (ok=%v)", wantAt, next, availableAt, ok)
	}
	if got := ProposeSession(user, utc); got != api.SessionTypeCheckin {
		t.Errorf("gated: want checkin proposed, got %s", got)
	}

	// An abandoned start (< 5 min) must not consume the deep-session slot.
	addSession(t, "u-1", string(api.SessionTypeSessionValues), now.Add(-time.Hour), 2)
	if next, _, _ := NextDeepSession(user, utc); next != api.SessionTypeSessionValues {
		t.Errorf("abandoned values session must not count as done, got next=%s", next)
	}

	// Gate open: a user whose movement was 6 days ago gets Values now.
	user2 := checkinUser("u-2")
	addSession(t, "u-2", string(api.SessionTypeOnboarding), now.Add(-31*24*time.Hour), 3)
	addSession(t, "u-2", string(api.SessionTypeSessionVision), now.Add(-30*24*time.Hour), 25)
	addSession(t, "u-2", string(api.SessionTypeSessionMovement), now.Add(-6*24*time.Hour), 20)
	next, availableAt, ok = NextDeepSession(user2, utc)
	if !ok || next != api.SessionTypeSessionValues || availableAt.After(now) {
		t.Errorf("gate open: want values available, got %s at %s (ok=%v)", next, availableAt, ok)
	}
	if got := ProposeSession(user2, utc); got != api.SessionTypeSessionValues {
		t.Errorf("gate open: want values proposed, got %s", got)
	}

	// Journey complete: the path cycles — the least recently done deep session
	// comes around again, 5 days after the most recent one. The opening pair
	// (intro + Vision) is done once and never cycles back.
	user3 := checkinUser("u-3")
	for i, st := range []api.SessionType{api.SessionTypeOnboarding, api.SessionTypeSessionVision, api.SessionTypeSessionMovement, api.SessionTypeSessionValues, api.SessionTypeSessionEnergy, api.SessionTypeSessionDecisions, api.SessionTypeSessionBeliefs, api.SessionTypeSessionIdentity, api.SessionTypeSessionAcceptance, api.SessionTypeSessionPriorities} {
		addSession(t, "u-3", string(st), now.Add(-time.Duration(77-i*7)*24*time.Hour), 20)
	}
	next, availableAt, ok = NextDeepSession(user3, utc)
	if !ok || next != api.SessionTypeSessionMovement || availableAt.After(now) {
		t.Errorf("completed journey: want movement cycling now, got %s at %s (ok=%v)", next, availableAt, ok)
	}
	if got := ProposeSession(user3, utc); got != api.SessionTypeSessionMovement {
		t.Errorf("completed journey: want movement proposed, got %s", got)
	}

	// The QA-reset shape: all deep sessions done historically, the opening pair
	// redone today. The cycle is gated 5 calendar days from the latest one, and the
	// preview must say so (this state used to return no session and no preview).
	user4 := checkinUser("u-4")
	for i, st := range []api.SessionType{api.SessionTypeSessionMovement, api.SessionTypeSessionValues, api.SessionTypeSessionEnergy, api.SessionTypeSessionDecisions, api.SessionTypeSessionBeliefs, api.SessionTypeSessionIdentity, api.SessionTypeSessionAcceptance, api.SessionTypeSessionPriorities} {
		addSession(t, "u-4", string(st), now.Add(-time.Duration(77-i*7)*24*time.Hour), 20)
	}
	redoneVision := startOfDay(now, utc).Add(time.Hour)
	addSession(t, "u-4", string(api.SessionTypeOnboarding), redoneVision, 3)
	addSession(t, "u-4", string(api.SessionTypeSessionVision), redoneVision, 25)
	next, availableAt, ok = NextDeepSession(user4, utc)
	wantAt = startOfDay(redoneVision, utc).AddDate(0, 0, 5)
	if !ok || next != api.SessionTypeSessionMovement || !availableAt.Equal(wantAt) {
		t.Errorf("post-reset: want movement at %s, got %s at %s (ok=%v)", wantAt, next, availableAt, ok)
	}
	if got := ProposeSession(user4, utc); got != api.SessionTypeCheckin {
		t.Errorf("post-reset: want checkin proposed while gated, got %s", got)
	}
}

func TestUpcomingDeepSessions(t *testing.T) {
	setupGrowthTest(t)
	now := time.Now()
	utc := time.UTC

	// Fresh user who completed Vision today
	user := checkinUser("u-upcoming")
	freshVision := startOfDay(now, utc).Add(2 * time.Hour)
	addSession(t, "u-upcoming", string(api.SessionTypeOnboarding), freshVision.Add(-time.Hour), 3)
	addSession(t, "u-upcoming", string(api.SessionTypeSessionVision), freshVision, 25)

	upcoming := UpcomingDeepSessions(user, utc)
	if len(upcoming) != 8 {
		t.Fatalf("expected 8 upcoming sessions, got %d", len(upcoming))
	}

	expectedSessions := []api.SessionType{
		api.SessionTypeSessionMovement,
		api.SessionTypeSessionValues,
		api.SessionTypeSessionEnergy,
		api.SessionTypeSessionDecisions,
		api.SessionTypeSessionBeliefs,
		api.SessionTypeSessionIdentity,
		api.SessionTypeSessionAcceptance,
		api.SessionTypeSessionPriorities,
	}

	for i, exp := range expectedSessions {
		if upcoming[i].Session != exp {
			t.Errorf("at index %d: expected session %s, got %s", i, exp, upcoming[i].Session)
		}
	}

	wantMovementAt := startOfDay(freshVision, utc).AddDate(0, 0, 1)
	if !upcoming[0].AvailableAt.Equal(wantMovementAt) {
		t.Errorf("movement availableAt: expected %s, got %s", wantMovementAt, upcoming[0].AvailableAt)
	}

	wantValuesAt := startOfDay(wantMovementAt, utc).AddDate(0, 0, 5)
	if !upcoming[1].AvailableAt.Equal(wantValuesAt) {
		t.Errorf("values availableAt: expected %s, got %s", wantValuesAt, upcoming[1].AvailableAt)
	}

	wantEnergyAt := startOfDay(wantValuesAt, utc).AddDate(0, 0, 5)
	if !upcoming[2].AvailableAt.Equal(wantEnergyAt) {
		t.Errorf("energy availableAt: expected %s, got %s", wantEnergyAt, upcoming[2].AvailableAt)
	}
}

// QA: /journey came back without "sessions" at all for a fresh account. Two causes,
// both of which this pins: the roadmap bailed out on the opening-pair states (and
// VISION_IDEAL_LIFE is the default for a new account, so it fired for everyone), and
// Vision was excluded from the list even though it is the first thing ahead.
func TestUpcomingDeepSessionsForFreshAccount(t *testing.T) {
	setupGrowthTest(t)
	utc := time.UTC

	// Exactly the QA user: default state, nothing done yet.
	state := string(models.StateVisionIdealLife)
	user := &models.User{ID: "u-fresh", State: &state}

	upcoming := UpcomingDeepSessions(user, utc)
	if len(upcoming) == 0 {
		t.Fatal("a fresh account must still get the road ahead, not an empty list")
	}

	// The whole journey, in order, starting with what is available now.
	want := []api.SessionType{
		api.SessionTypeSessionVision, api.SessionTypeSessionMovement, api.SessionTypeSessionValues,
		api.SessionTypeSessionEnergy, api.SessionTypeSessionDecisions, api.SessionTypeSessionBeliefs,
		api.SessionTypeSessionIdentity, api.SessionTypeSessionAcceptance, api.SessionTypeSessionPriorities,
	}
	if len(upcoming) != len(want) {
		t.Fatalf("roadmap has %d entries, want %d: %+v", len(upcoming), len(want), upcoming)
	}
	for i, w := range want {
		if upcoming[i].Session != w {
			t.Errorf("roadmap[%d] = %s, want %s", i, upcoming[i].Session, w)
		}
	}

	// Vision is available now; Movement must NOT be — it only unlocks the day after
	// Vision is done. Dropping Vision from the list used to make Movement claim today.
	if upcoming[0].AvailableAt.After(time.Now()) {
		t.Errorf("vision should be available now, got %s", upcoming[0].AvailableAt)
	}
	if !upcoming[1].AvailableAt.After(time.Now()) {
		t.Errorf("movement must not be available today, got %s", upcoming[1].AvailableAt)
	}
	// Dates must be strictly increasing down the road.
	for i := 1; i < len(upcoming); i++ {
		if !upcoming[i].AvailableAt.After(upcoming[i-1].AvailableAt) {
			t.Errorf("roadmap dates not increasing at %d: %s then %s",
				i, upcoming[i-1].AvailableAt, upcoming[i].AvailableAt)
		}
	}
}

// Once Vision is done it drops off the road ahead, and Movement leads.
func TestUpcomingDeepSessionsAfterVision(t *testing.T) {
	setupGrowthTest(t)
	utc := time.UTC
	now := time.Now()

	user := checkinUser("u-after")
	addSession(t, "u-after", string(api.SessionTypeOnboarding), now.AddDate(0, 0, -3), 3)
	addSession(t, "u-after", string(api.SessionTypeSessionVision), now.AddDate(0, 0, -2), 25)

	upcoming := UpcomingDeepSessions(user, utc)
	if len(upcoming) == 0 {
		t.Fatal("expected a road ahead after Vision")
	}
	for _, u := range upcoming {
		if u.Session == api.SessionTypeSessionVision {
			t.Error("a completed Vision must not still appear ahead")
		}
	}
	if upcoming[0].Session != api.SessionTypeSessionMovement {
		t.Errorf("roadmap should lead with movement, got %s", upcoming[0].Session)
	}
}
