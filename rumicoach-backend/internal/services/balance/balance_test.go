package balance

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created tables (like user_test.go): the models' Postgres column types
	// ("timestamp with time zone") get TEXT affinity under SQLite AutoMigrate and
	// break time.Time scans; DATETIME columns scan fine.
	for _, ddl := range []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			state TEXT,
			date_of_birth DATE,
			gender TEXT,
			country TEXT,
			ideal_life_vision_set_at DATETIME,
			balance_seconds INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME
		)`,
		`CREATE TABLE balance_transactions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			amount_seconds INTEGER NOT NULL,
			balance_after INTEGER NOT NULL,
			session_id TEXT UNIQUE,
			session_type TEXT,
			product TEXT,
			reference_id TEXT UNIQUE,
			description TEXT,
			created_at DATETIME
		)`,
		`CREATE TABLE communication_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			session_type TEXT,
			start_time DATETIME NOT NULL,
			duration INTEGER
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
	}
	database.DB = db
}

func createUser(t *testing.T, id string) {
	t.Helper()
	// Raw insert: GORM's Create writes every model column, and the slim test
	// table only carries the ones this package touches.
	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES (?, 0)`, id).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
}

func userBalance(t *testing.T, id string) int64 {
	t.Helper()
	var user models.User
	if err := database.DB.First(&user, "id = ?", id).Error; err != nil {
		t.Fatalf("failed to load user: %v", err)
	}
	return user.BalanceSeconds
}

func strPtr(s string) *string { return &s }

func TestCreditAndDebitChainBalance(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	entry, err := Credit(ctx, "u1", 7200, models.BalanceTxSubscription, strPtr("monthly_120"), nil)
	if err != nil {
		t.Fatalf("credit failed: %v", err)
	}
	if entry.AmountSeconds != 7200 || entry.BalanceAfter != 7200 {
		t.Errorf("credit entry: got amount=%d balanceAfter=%d, want 7200/7200", entry.AmountSeconds, entry.BalanceAfter)
	}

	debit, err := DebitSession(ctx, "u1", "sess-1", nil, 300)
	if err != nil {
		t.Fatalf("debit failed: %v", err)
	}
	if debit.AmountSeconds != -300 || debit.BalanceAfter != 6900 {
		t.Errorf("debit entry: got amount=%d balanceAfter=%d, want -300/6900", debit.AmountSeconds, debit.BalanceAfter)
	}
	if debit.SessionID == nil || *debit.SessionID != "sess-1" {
		t.Errorf("debit entry sessionID: got %v, want sess-1", debit.SessionID)
	}
	if got := userBalance(t, "u1"); got != 6900 {
		t.Errorf("user balance: got %d, want 6900", got)
	}
}

func TestDebitPastZeroGoesNegative(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")

	entry, err := DebitSession(context.Background(), "u1", "sess-1", nil, 90)
	if err != nil {
		t.Fatalf("debit failed: %v", err)
	}
	if entry.BalanceAfter != -90 {
		t.Errorf("balanceAfter: got %d, want -90", entry.BalanceAfter)
	}
	if got := userBalance(t, "u1"); got != -90 {
		t.Errorf("user balance: got %d, want -90", got)
	}
}

func TestDebitSameSessionTwiceFails(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	if _, err := DebitSession(ctx, "u1", "sess-1", nil, 60); err != nil {
		t.Fatalf("first debit failed: %v", err)
	}
	if _, err := DebitSession(ctx, "u1", "sess-1", nil, 60); err == nil {
		t.Fatal("second debit for the same session succeeded, want unique-index error")
	}
	if got := userBalance(t, "u1"); got != -60 {
		t.Errorf("user balance after failed double debit: got %d, want -60", got)
	}
}

func TestDebitNonPositiveElapsedIsNoOp(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	for _, elapsed := range []int64{0, -5} {
		entry, err := DebitSession(ctx, "u1", "sess-1", nil, elapsed)
		if err != nil || entry != nil {
			t.Errorf("elapsed=%d: got entry=%v err=%v, want nil/nil", elapsed, entry, err)
		}
	}
	var count int64
	database.DB.Model(&models.BalanceTransaction{}).Count(&count)
	if count != 0 {
		t.Errorf("ledger rows: got %d, want 0", count)
	}
}

func TestCreditValidation(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	if _, err := Credit(ctx, "u1", 0, models.BalanceTxTopUp, nil, nil); err == nil {
		t.Error("credit of 0 succeeded, want error")
	}
	if _, err := Credit(ctx, "u1", -60, models.BalanceTxTopUp, nil, nil); err == nil {
		t.Error("negative credit succeeded, want error")
	}
	if _, err := Credit(ctx, "u1", 60, models.BalanceTxSessionUsage, nil, nil); err == nil {
		t.Error("credit with session_usage type succeeded, want error")
	}
}

func TestUnknownUser(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	if _, err := Credit(ctx, "missing", 60, models.BalanceTxTopUp, nil, nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("credit unknown user: got %v, want ErrRecordNotFound", err)
	}
	if _, err := DebitSession(ctx, "missing", "sess-1", nil, 60); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("debit unknown user: got %v, want ErrRecordNotFound", err)
	}
}

func TestListTransactionsNewestFirstPaginated(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	createUser(t, "u2")
	ctx := context.Background()

	// Interleave another user's rows to prove filtering.
	for i, sessID := range []string{"a", "b", "c"} {
		if _, err := Credit(ctx, "u1", int64(60*(i+1)), models.BalanceTxTopUp, nil, nil); err != nil {
			t.Fatalf("credit failed: %v", err)
		}
		if _, err := DebitSession(ctx, "u2", "other-"+sessID, nil, 10); err != nil {
			t.Fatalf("debit failed: %v", err)
		}
	}

	items, total, err := ListTransactions(ctx, "u1", 1, 2)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if len(items) != 2 {
		t.Fatalf("page size: got %d, want 2", len(items))
	}
	// Newest first: the 180s credit was the last written for u1.
	if items[0].AmountSeconds != 180 {
		t.Errorf("first item amount: got %d, want 180 (newest first)", items[0].AmountSeconds)
	}
	for _, it := range items {
		if it.UserID != "u1" {
			t.Errorf("leaked row for user %s", it.UserID)
		}
	}

	page2, _, err := ListTransactions(ctx, "u1", 2, 2)
	if err != nil {
		t.Fatalf("list page 2 failed: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("page 2 size: got %d, want 1", len(page2))
	}
}

func TestCreditPurchaseDedupesOnReference(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	entry, err := CreditPurchase(ctx, "u1", 7200, models.BalanceTxSubscription, strPtr("monthly_120"), nil, "revenuecat:evt-1")
	if err != nil {
		t.Fatalf("first credit failed: %v", err)
	}
	if entry.ReferenceID == nil || *entry.ReferenceID != "revenuecat:evt-1" {
		t.Errorf("referenceID: got %v, want revenuecat:evt-1", entry.ReferenceID)
	}

	if _, err := CreditPurchase(ctx, "u1", 7200, models.BalanceTxSubscription, strPtr("monthly_120"), nil, "revenuecat:evt-1"); !errors.Is(err, ErrDuplicateReference) {
		t.Fatalf("second credit: got err %v, want ErrDuplicateReference", err)
	}
	if got := userBalance(t, "u1"); got != 7200 {
		t.Errorf("user balance after duplicate: got %d, want 7200", got)
	}

	if _, err := CreditPurchase(ctx, "u1", 3600, models.BalanceTxTopUp, nil, nil, "revenuecat:evt-2"); err != nil {
		t.Fatalf("distinct reference credit failed: %v", err)
	}
	if got := userBalance(t, "u1"); got != 10800 {
		t.Errorf("user balance: got %d, want 10800", got)
	}

	if _, err := CreditPurchase(ctx, "u1", 3600, models.BalanceTxTopUp, nil, nil, ""); err == nil {
		t.Fatal("empty reference accepted, want error")
	}
}

// addSession inserts a session row of the given type and duration. Duration is what
// separates a session that happened from a connection that was dropped, so it is
// always explicit here.
func addSession(t *testing.T, id, userID, sessionType string, durationSeconds int) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO communication_sessions (id, user_id, session_type, start_time, duration) VALUES (?, ?, ?, ?, ?)`,
		id, userID, sessionType, time.Now(), durationSeconds).Error; err != nil {
		t.Fatalf("failed to insert session %s: %v", id, err)
	}
}

// completeProfile fills in the three details the onboarding intro collects.
func completeProfile(t *testing.T, userID string) {
	t.Helper()
	if err := database.DB.Exec(
		`UPDATE users SET date_of_birth = '1990-05-03', gender = 'male', country = 'PT' WHERE id = ?`,
		userID).Error; err != nil {
		t.Fatalf("failed to complete profile for %s: %v", userID, err)
	}
}

// setVision records the ideal-life vision, which is what the Vision session exists to
// produce and therefore what ends the introductory allowance.
func setVision(t *testing.T, userID string) {
	t.Helper()
	if err := database.DB.Exec(
		`UPDATE users SET ideal_life_vision_set_at = ? WHERE id = ?`, time.Now(), userID).Error; err != nil {
		t.Fatalf("failed to set vision for %s: %v", userID, err)
	}
}

// The allowance follows the two artifacts the opening pair exists to produce, so it
// ends when the introduction is actually over and not before.
func TestOpeningPairUnfinishedFollowsTheArtifacts(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	ctx := context.Background()

	// Brand-new account: nothing collected, nothing written. The intro must be free
	// or a zero-balance account could never begin.
	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || !free {
		t.Fatalf("fresh account: got (%v, %v), want (true, nil)", free, err)
	}

	// The intro has run and saved the details, but Vision has not written anything —
	// which is precisely the session the user is about to start.
	completeProfile(t, "u1")
	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || !free {
		t.Fatalf("profile done, vision missing: got (%v, %v), want (true, nil)", free, err)
	}

	// Both on record: the introduction is over and the balance applies from here.
	setVision(t, "u1")
	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || free {
		t.Fatalf("both artifacts on record: got (%v, %v), want (false, nil)", free, err)
	}
}

// The regression this rule was written for: the session row is inserted when the
// socket opens, so counting rows charged users for connections that produced nothing.
// Five seconds of Vision used to spend the last free session and drop the user onto a
// paywall they could not get past to finish being onboarded.
func TestOpeningPairUnfinishedIgnoresAbandonedStarts(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	completeProfile(t, "u1")
	ctx := context.Background()

	addSession(t, "intro", "u1", "onboarding", 90)
	addSession(t, "dropped", "u1", "session_vision", 5)

	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || !free {
		t.Fatalf("after a 5-second Vision drop: got (%v, %v), want (true, nil)", free, err)
	}
}

// A user who never lets Vision finish never stops qualifying, so the artifact rule is
// backstopped by a cap. Only sessions that really happened count towards it.
func TestOpeningPairUnfinishedCapsUnfinishedIntroductions(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	completeProfile(t, "u1")
	ctx := context.Background()

	// Well past the cap in rows, but every one of them was abandoned.
	for i := 0; i < int(FreeSessionCap)+5; i++ {
		addSession(t, fmt.Sprintf("drop%d", i), "u1", "session_vision", 3)
	}
	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || !free {
		t.Fatalf("abandoned starts must not fill the cap: got (%v, %v), want (true, nil)", free, err)
	}

	// Real sessions do fill it, vision or no vision.
	for i := 0; i < int(FreeSessionCap); i++ {
		addSession(t, fmt.Sprintf("real%d", i), "u1", "session_vision", 600)
	}
	if free, err := OpeningPairUnfinished(ctx, "u1"); err != nil || free {
		t.Fatalf("at the cap: got (%v, %v), want (false, nil)", free, err)
	}
}

// The allowance is per user. A shared count would put everybody past it the moment the
// product had any traffic at all.
func TestOpeningPairUnfinishedIsPerUser(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	createUser(t, "u2")
	ctx := context.Background()

	completeProfile(t, "u1")
	setVision(t, "u1")
	for i := 0; i < int(FreeSessionCap); i++ {
		addSession(t, fmt.Sprintf("a%d", i), "u1", "checkin", 600)
	}

	if free, err := OpeningPairUnfinished(ctx, "u2"); err != nil || !free {
		t.Fatalf("untouched user: got (%v, %v), want (true, nil)", free, err)
	}
}

// A user who does not exist is not a user owed free sessions. The WebSocket gate reads
// the error and fails open on its own terms; nothing here may report "free" for a row
// it could not find.
func TestOpeningPairUnfinishedUnknownUser(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	if free, err := OpeningPairUnfinished(ctx, "nobody"); err == nil || free {
		t.Fatalf("unknown user: got (%v, %v), want (false, error)", free, err)
	}
}

// The allowance covers two session types and no others. A check-in is paid for out of
// the balance whatever state the account is in — without this, a user whose Vision was
// cut short (which parks them at CHECKIN) got free check-ins until the cap.
func TestFreeSessionAvailableOnlyCoversIntroductoryTypes(t *testing.T) {
	setupTestDB(t)
	createUser(t, "u1")
	completeProfile(t, "u1") // opening pair unfinished: the vision is still missing
	ctx := context.Background()

	for _, c := range []struct {
		sessionType string
		want        bool
	}{
		{"onboarding", true},
		{"session_vision", true},
		{"checkin", false},
		{"session_movement", false},
		{"session_values", false},
		{"", false},
	} {
		if free, err := FreeSessionAvailable(ctx, "u1", c.sessionType); err != nil || free != c.want {
			t.Errorf("%q with an unfinished opening pair: got (%v, %v), want (%v, nil)",
				c.sessionType, free, err, c.want)
		}
	}

	// Once the pair is done, even its own two types are billable.
	setVision(t, "u1")
	for _, sessionType := range []string{"onboarding", "session_vision"} {
		if free, err := FreeSessionAvailable(ctx, "u1", sessionType); err != nil || free {
			t.Errorf("%q after the pair is finished: got (%v, %v), want (false, nil)", sessionType, free, err)
		}
	}
}

// A billable type is settled on the type alone — no user row is read, so a database
// that cannot answer cannot turn a check-in into a free session.
func TestFreeSessionAvailableBillableTypeNeedsNoUser(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	if free, err := FreeSessionAvailable(ctx, "nobody", "checkin"); err != nil || free {
		t.Fatalf("checkin for an unknown user: got (%v, %v), want (false, nil)", free, err)
	}
}
