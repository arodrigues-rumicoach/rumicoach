package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/services/usage"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUsageTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Hand-created tables (like balance_test.go): the models' Postgres column
	// types get TEXT affinity under SQLite AutoMigrate and break time.Time
	// scans; DATETIME columns scan fine.
	for _, ddl := range []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			state TEXT,
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
			start_time DATETIME,
			end_time DATETIME,
			duration INTEGER DEFAULT 0
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
	}
	database.DB = db
}

func insertUsageDebit(t *testing.T, id, userID, sessionID, sessionType string, seconds int64, at time.Time) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at)
		 VALUES (?, ?, 'session_usage', ?, 0, ?, ?, ?)`,
		id, userID, -seconds, sessionID, sessionType, at).Error; err != nil {
		t.Fatalf("failed to insert debit: %v", err)
	}
}

// insertMessageDebit seeds the charge for one reply. Counts and cost come from
// different tables now — the count from the outbound message event, the seconds from
// this — so a test that wants both has to seed both.
func insertMessageDebit(t *testing.T, id, userID string, at time.Time) {
	t.Helper()
	if err := database.DB.Exec(
		`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, reference_id, created_at)
		 VALUES (?, ?, 'message_usage', -5, 0, ?, ?)`,
		id, userID, "message:"+id, at).Error; err != nil {
		t.Fatalf("failed to insert message debit: %v", err)
	}
}

func TestGetUsage(t *testing.T) {
	setupUsageTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-123"

	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES (?, 0)`, userID).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	getUsage := func(t *testing.T, query, timezone string, params api.GetUsageParams) api.UsageHistoryResponse {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/me/usage"+query, nil)
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		if timezone != "" {
			req.Header.Set("X-Timezone", timezone)
		}
		w := httptest.NewRecorder()
		server.GetUsage(w, req, params)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp api.UsageHistoryResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		return resp
	}

	t.Run("Unauthorized request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/me/usage", nil)
		w := httptest.NewRecorder()
		server.GetUsage(w, req, api.GetUsageParams{})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Empty history", func(t *testing.T) {
		resp := getUsage(t, "", "", api.GetUsageParams{})
		if resp.Items == nil || len(*resp.Items) != 0 {
			t.Errorf("Expected empty items, got %v", resp.Items)
		}
		if *resp.Totals.TotalSeconds != 0 {
			t.Errorf("Expected zero totals, got %d", *resp.Totals.TotalSeconds)
		}
	})

	// Fixture: one credit (must NOT appear — the feed is usage, not the full
	// ledger), two session debits, and messages spread over two UTC days plus
	// noise rows (inbound, failed) that must not be counted.
	day1Session := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	day1Msg := time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC)
	day2Msg1 := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	day2Msg2 := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	day2Session := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	if err := database.DB.Exec(
		`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, created_at)
		 VALUES ('tx-credit', ?, 'subscription', 7200, 7200, ?)`, userID, day1Session.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("failed to insert credit: %v", err)
	}
	insertUsageDebit(t, "tx-s1", userID, "sess-1", "session_movement", 300, day1Session)
	insertUsageDebit(t, "tx-s2", userID, "sess-2", "checkin", 120, day2Session)
	// Three charged replies — the only thing the feed lists. Unprompted sends
	// (nudges, templates) produce no debit and so, by construction, no entry.
	insertMessageDebit(t, "d1", userID, day1Msg)
	insertMessageDebit(t, "d2", userID, day2Msg1)
	insertMessageDebit(t, "d3", userID, day2Msg2)

	t.Run("Merged feed newest first with day-grouped messages", func(t *testing.T) {
		resp := getUsage(t, "", "", api.GetUsageParams{})
		items := *resp.Items
		if len(items) != 4 {
			t.Fatalf("Expected 4 entries, got %d: %+v", len(items), items)
		}

		// Newest first: day2 session, day2 message group, day1 message group, day1 session.
		if items[0].Kind != "session" || *items[0].SessionId != "sess-2" || items[0].Seconds != 120 {
			t.Errorf("entry 0: expected sess-2/120s, got %+v", items[0])
		}
		if items[1].Kind != "messages" || *items[1].Date != "2026-08-15" || *items[1].MessageCount != 2 || items[1].Seconds != 10 {
			t.Errorf("entry 1: expected 2026-08-15 messages x2 = 10s, got %+v", items[1])
		}
		if items[2].Kind != "messages" || *items[2].Date != "2026-08-14" || *items[2].MessageCount != 1 || items[2].Seconds != 5 {
			t.Errorf("entry 2: expected 2026-08-14 messages x1 = 5s, got %+v", items[2])
		}
		if items[3].Kind != "session" || *items[3].SessionId != "sess-1" || items[3].Seconds != 300 {
			t.Errorf("entry 3: expected sess-1/300s, got %+v", items[3])
		}
		if *items[3].SessionType != "session_movement" {
			t.Errorf("entry 3: expected sessionType session_movement, got %v", items[3].SessionType)
		}

		if *resp.Totals.SessionSeconds != 420 || *resp.Totals.SessionCount != 2 {
			t.Errorf("Expected session totals 420s/2, got %d/%d", *resp.Totals.SessionSeconds, *resp.Totals.SessionCount)
		}
		if *resp.Totals.MessageSeconds != 15 || *resp.Totals.MessageCount != 3 {
			t.Errorf("Expected message totals 15s/3, got %d/%d", *resp.Totals.MessageSeconds, *resp.Totals.MessageCount)
		}
		if *resp.Totals.TotalSeconds != 435 {
			t.Errorf("Expected total 435s, got %d", *resp.Totals.TotalSeconds)
		}
	})

	t.Run("Timezone shifts the day grouping", func(t *testing.T) {
		// In Los Angeles (UTC-7), 14th 23:30 UTC and 15th 01:00 UTC are both the
		// 14th local, while 15th 09:00 UTC stays the 15th — so the groups become
		// 15th x1 and 14th x2, the reverse of the UTC split above.
		resp := getUsage(t, "", "America/Los_Angeles", api.GetUsageParams{})
		items := *resp.Items
		if len(items) != 4 {
			t.Fatalf("Expected 4 entries, got %d: %+v", len(items), items)
		}
		if items[1].Kind != "messages" || *items[1].Date != "2026-08-15" || *items[1].MessageCount != 1 || items[1].Seconds != 5 {
			t.Errorf("entry 1: expected 2026-08-15 messages x1 = 5s, got %+v", items[1])
		}
		if items[2].Kind != "messages" || *items[2].Date != "2026-08-14" || *items[2].MessageCount != 2 || items[2].Seconds != 10 {
			t.Errorf("entry 2: expected 2026-08-14 messages x2 = 10s, got %+v", items[2])
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		page, limit := 2, 3
		resp := getUsage(t, "?page=2&limit=3", "", api.GetUsageParams{Page: &page, Limit: &limit})
		items := *resp.Items
		if len(items) != 1 {
			t.Fatalf("Expected 1 entry on page 2, got %d", len(items))
		}
		if items[0].Kind != "session" || *items[0].SessionId != "sess-1" {
			t.Errorf("Expected sess-1 on page 2, got %+v", items[0])
		}
		if *resp.Pagination.TotalItems != 4 || *resp.Pagination.TotalPages != 2 {
			t.Errorf("Expected 4 items / 2 pages, got %d/%d", *resp.Pagination.TotalItems, *resp.Pagination.TotalPages)
		}
	})
}

// The free introductory sessions appear in the history, with their real duration and
// marked free. Hiding them would describe an account the user does not have: their
// first two sessions are exactly the ones missing, and a history that starts at
// session three looks broken rather than generous.
func TestGetUsageShowsFreeSessions(t *testing.T) {
	setupUsageTestDB(t)

	// A free session that ran for 10 minutes, and a paid one debited 300 seconds.
	database.DB.Exec(`INSERT INTO communication_sessions (id, user_id, session_type, duration) VALUES ('s-free','u1','session_vision',600)`)
	database.DB.Exec(
		`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at) VALUES
		 ('t1','u1','session_free',0,0,'s-free','session_vision',?),
		 ('t2','u1','session_usage',-300,600,'s-paid','checkin',?)`,
		time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))

	entries, totals, err := usage.History(context.Background(), "u1", time.UTC)
	if err != nil {
		t.Fatalf("history failed: %v", err)
	}

	var free, paid *usage.Entry
	for i := range entries {
		switch {
		case entries[i].SessionID != nil && *entries[i].SessionID == "s-free":
			free = &entries[i]
		case entries[i].SessionID != nil && *entries[i].SessionID == "s-paid":
			paid = &entries[i]
		}
	}
	if free == nil {
		t.Fatal("the free session is missing from the usage history")
	}
	if !free.Free {
		t.Error("the free session is not marked free")
	}
	// The real duration, not the zero amount on the ledger row: "0 minutes" for a
	// ten-minute session would read as nothing having happened.
	if free.Seconds != 600 {
		t.Errorf("free session duration = %d, want 600", free.Seconds)
	}
	if paid == nil || paid.Free || paid.Seconds != 300 {
		t.Errorf("paid session = %+v, want 300 seconds and not free", paid)
	}

	// Totals keep the two apart: sessionSeconds is what was actually charged, so a
	// free session must not inflate it, and totalSeconds is what the user has spent.
	if totals.SessionSeconds != 300 {
		t.Errorf("sessionSeconds = %d, want 300 (free time excluded)", totals.SessionSeconds)
	}
	if totals.FreeSessionSeconds != 600 {
		t.Errorf("freeSessionSeconds = %d, want 600", totals.FreeSessionSeconds)
	}
	if totals.TotalSeconds != 300 {
		t.Errorf("totalSeconds = %d, want 300 (free time is not spend)", totals.TotalSeconds)
	}
	// Both are sessions the user had.
	if totals.SessionCount != 2 {
		t.Errorf("sessionCount = %d, want 2", totals.SessionCount)
	}
}

// A free session whose communication_sessions row is gone still appears, at zero
// seconds. Dropping the entry would lose the only trace the user has of it.
func TestGetUsageFreeSessionWithoutItsSessionRow(t *testing.T) {
	setupUsageTestDB(t)
	database.DB.Exec(
		`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at) VALUES
		 ('t1','u1','session_free',0,0,'vanished','onboarding',?)`, time.Now())

	entries, totals, err := usage.History(context.Background(), "u1", time.UTC)
	if err != nil {
		t.Fatalf("history failed: %v", err)
	}
	if len(entries) != 1 || !entries[0].Free || entries[0].Seconds != 0 {
		t.Fatalf("entries = %+v, want one free entry at 0 seconds", entries)
	}
	if totals.SessionCount != 1 {
		t.Errorf("sessionCount = %d, want 1", totals.SessionCount)
	}
}
