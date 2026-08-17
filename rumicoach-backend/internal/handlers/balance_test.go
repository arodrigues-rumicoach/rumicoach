package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBalanceTestDB(t *testing.T) {
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

func TestGetTransactions(t *testing.T) {
	setupBalanceTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-123"

	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES (?, 0)`, userID).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	t.Run("Unauthorized request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/me/transactions", nil)
		w := httptest.NewRecorder()

		server.GetTransactions(w, req, api.GetTransactionsParams{})

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Empty ledger", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/me/transactions", nil)
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		w := httptest.NewRecorder()

		server.GetTransactions(w, req, api.GetTransactionsParams{})

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}
		var resp api.BalanceTransactionPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Items == nil || len(*resp.Items) != 0 {
			t.Errorf("Expected empty items, got %v", resp.Items)
		}
	})

	t.Run("Newest first with pagination", func(t *testing.T) {
		if _, err := balance.Credit(t.Context(), userID, 3600, models.BalanceTxSubscription, nil, nil); err != nil {
			t.Fatalf("credit failed: %v", err)
		}
		if _, err := balance.DebitSession(t.Context(), userID, "sess-1", nil, 120); err != nil {
			t.Fatalf("debit failed: %v", err)
		}

		page := 1
		limit := 1
		req := httptest.NewRequest("GET", "/v1/me/transactions?page=1&limit=1", nil)
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		w := httptest.NewRecorder()

		server.GetTransactions(w, req, api.GetTransactionsParams{Page: &page, Limit: &limit})

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}
		var resp api.BalanceTransactionPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		items := *resp.Items
		if len(items) != 1 {
			t.Fatalf("Expected 1 item, got %d", len(items))
		}
		// Newest first: the debit was written last.
		if *items[0].AmountSeconds != -120 {
			t.Errorf("Expected newest item amount -120, got %d", *items[0].AmountSeconds)
		}
		if *items[0].SessionId != "sess-1" {
			t.Errorf("Expected sessionId sess-1, got %v", items[0].SessionId)
		}
		if *resp.Pagination.TotalItems != 2 {
			t.Errorf("Expected 2 total items, got %d", *resp.Pagination.TotalItems)
		}
		if *resp.Pagination.TotalPages != 2 {
			t.Errorf("Expected 2 total pages, got %d", *resp.Pagination.TotalPages)
		}
	})
}

func TestGetTransactionsGrouped(t *testing.T) {
	setupBalanceTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-123"

	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES (?, 3460)`, userID).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Explicit rows rather than the balance helpers: the fold is decided by
	// created_at ordering, so the times must be fixed, not time.Now().
	rows := []struct {
		id, txType, sessionID, sessionType string
		amount, balanceAfter               int64
		createdAt                          string
	}{
		{"tx-1", "subscription", "", "", 3600, 3600, "2024-05-01 10:00:00"},
		{"tx-2", "message_usage", "", "", -5, 3595, "2024-05-02 09:00:00"},
		{"tx-3", "message_usage", "", "", -5, 3590, "2024-05-02 09:05:00"},
		{"tx-4", "session_usage", "sess-1", "checkin", -120, 3470, "2024-05-02 12:00:00"},
		{"tx-5", "message_usage", "", "", -5, 3465, "2024-05-02 15:00:00"},
		{"tx-6", "message_usage", "", "", -5, 3460, "2024-05-03 08:00:00"},
		{"tx-7", "session_free", "sess-free-1", "onboarding", 0, 3460, "2024-05-03 08:30:00"},
	}
	for _, r := range rows {
		var sessionID, sessionType any
		if r.sessionID != "" {
			sessionID, sessionType = r.sessionID, r.sessionType
		}
		if err := database.DB.Exec(
			`INSERT INTO balance_transactions (id, user_id, type, amount_seconds, balance_after, session_id, session_type, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, userID, r.txType, r.amount, r.balanceAfter, sessionID, sessionType, r.createdAt,
		).Error; err != nil {
			t.Fatalf("failed to insert ledger row %s: %v", r.id, err)
		}
	}

	get := func(t *testing.T, timezone string) []api.BalanceTransaction {
		t.Helper()
		grouped := true
		req := httptest.NewRequest("GET", "/v1/me/transactions?grouped=true", nil)
		if timezone != "" {
			req.Header.Set("X-Timezone", timezone)
		}
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		w := httptest.NewRecorder()
		server.GetTransactions(w, req, api.GetTransactionsParams{Grouped: &grouped})
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp api.BalanceTransactionPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		return *resp.Items
	}

	t.Run("Folds same-day runs, keeps interleaved rows separate", func(t *testing.T) {
		items := get(t, "")

		// 7 ledger rows → 6 statement rows: only tx-2+tx-3 fold (same day AND
		// consecutive). tx-5 is the same day as them but the session debit tx-4
		// broke the run, and tx-6 is another day.
		if len(items) != 6 {
			t.Fatalf("Expected 6 statement rows, got %d", len(items))
		}

		if string(*items[0].Type) != "session_free" || *items[0].AmountSeconds != 0 {
			t.Errorf("Expected session_free with amount 0 first, got %s/%d", *items[0].Type, *items[0].AmountSeconds)
		}
		if items[1].Day == nil || *items[1].Day != "2024-05-03" || *items[1].MessageCount != 1 {
			t.Errorf("Expected single-message group for 2024-05-03, got %+v", items[1])
		}
		if *items[2].MessageCount != 1 || *items[2].AmountSeconds != -5 || *items[2].BalanceAfter != 3465 {
			t.Errorf("Expected tx-5 alone (run broken by session debit), got %+v", items[2])
		}
		if string(*items[3].Type) != "session_usage" || *items[3].AmountSeconds != -120 {
			t.Errorf("Expected session debit, got %+v", items[3])
		}
		// The folded run: summed amount, count 2, and the NEWEST row's
		// id/balanceAfter/createdAt standing for the group.
		if *items[4].Id != "tx-3" || *items[4].AmountSeconds != -10 || *items[4].MessageCount != 2 || *items[4].BalanceAfter != 3590 {
			t.Errorf("Expected tx-2+tx-3 folded (id tx-3, -10, count 2, balance 3590), got %+v", items[4])
		}
		if string(*items[5].Type) != "subscription" || *items[5].AmountSeconds != 3600 {
			t.Errorf("Expected the credit last, got %+v", items[5])
		}

		// Non-group rows carry no group fields.
		if items[0].MessageCount != nil || items[3].Day != nil {
			t.Errorf("Expected group fields only on message groups")
		}
	})

	t.Run("X-Timezone moves the day boundary", func(t *testing.T) {
		// In Auckland (+12), tx-5 (15:00 UTC) and tx-6 (08:00 UTC next day) are
		// the same local day and consecutive, so they fold: 5 rows, not 6.
		items := get(t, "Pacific/Auckland")
		if len(items) != 5 {
			t.Fatalf("Expected 5 statement rows in Auckland time, got %d", len(items))
		}
		if *items[1].MessageCount != 2 || *items[1].AmountSeconds != -10 || *items[1].BalanceAfter != 3460 || *items[1].Day != "2024-05-03" {
			t.Errorf("Expected tx-5+tx-6 folded in Auckland time, got %+v", items[1])
		}
	})

	t.Run("Ungrouped stays raw", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/me/transactions", nil)
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		w := httptest.NewRecorder()
		server.GetTransactions(w, req, api.GetTransactionsParams{})
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}
		var resp api.BalanceTransactionPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(*resp.Items) != 7 {
			t.Errorf("Expected 7 raw rows, got %d", len(*resp.Items))
		}
	})
}

func TestPostAdminUsersIdCredits(t *testing.T) {
	setupBalanceTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-123"

	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES (?, 0)`, userID).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	post := func(t *testing.T, targetUser, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/admin/users/"+targetUser+"/credits", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		server.PostAdminUsersIdCredits(w, req, targetUser)
		return w
	}

	t.Run("Credit with minutes", func(t *testing.T) {
		w := post(t, userID, `{"amountMinutes": 120, "type": "subscription", "product": "monthly_120"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}
		var resp api.BalanceTransaction
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if *resp.AmountSeconds != 7200 || *resp.BalanceAfter != 7200 {
			t.Errorf("Expected 7200/7200, got %d/%d", *resp.AmountSeconds, *resp.BalanceAfter)
		}
		if *resp.Product != "monthly_120" {
			t.Errorf("Expected product monthly_120, got %v", resp.Product)
		}
		// The cadence enum is derived server-side from the product id, so
		// clients translate instead of parsing slugs.
		if resp.Plan == nil || *resp.Plan != api.Monthly {
			t.Errorf("Expected plan monthly, got %v", resp.Plan)
		}
	})

	t.Run("Credit with seconds", func(t *testing.T) {
		w := post(t, userID, `{"amountSeconds": 600, "type": "top_up"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
		}
		var resp api.BalanceTransaction
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if *resp.BalanceAfter != 7800 {
			t.Errorf("Expected balanceAfter 7800, got %d", *resp.BalanceAfter)
		}
	})

	t.Run("Both amount fields rejected", func(t *testing.T) {
		w := post(t, userID, `{"amountSeconds": 600, "amountMinutes": 10, "type": "top_up"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Neither amount field rejected", func(t *testing.T) {
		w := post(t, userID, `{"type": "top_up"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Non-positive amount rejected", func(t *testing.T) {
		w := post(t, userID, `{"amountSeconds": -60, "type": "top_up"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid type rejected", func(t *testing.T) {
		w := post(t, userID, `{"amountSeconds": 60, "type": "session_usage"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Unknown user", func(t *testing.T) {
		w := post(t, "missing-user", `{"amountSeconds": 60, "type": "top_up"}`)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})
}
