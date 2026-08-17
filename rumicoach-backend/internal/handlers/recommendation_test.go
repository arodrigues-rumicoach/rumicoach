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
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetRecommendations(t *testing.T) {
	// Initialize in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db

	// Create table manually with standard DATETIME type to avoid SQLite parsing issue
	// with "timestamp with time zone" PostgreSQL-specific types.
	err = db.Exec(`CREATE TABLE recommendations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		title TEXT NOT NULL,
		type TEXT NOT NULL,
		author TEXT,
		url TEXT,
		description TEXT NOT NULL,
		created_at DATETIME NOT NULL
	)`).Error
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	logger := zap.NewNop()
	server := NewServer(logger, nil, nil, nil)

	// Create test recommendations for two different users
	user1 := "user123"
	user2 := "user456"

	rec1 := models.Recommendation{
		ID:          "rec-1",
		UserID:      user1,
		SessionID:   "session-1",
		Title:       "Deep Work",
		Type:        "book",
		Author:      func(s string) *string { return &s }("Cal Newport"),
		URL:         func(s string) *string { return &s }("https://example.com/deepwork"),
		Description: "A great book about distraction-free focus.",
		CreatedAt:   time.Now().Add(-2 * time.Hour),
	}
	rec2 := models.Recommendation{
		ID:          "rec-2",
		UserID:      user1,
		SessionID:   "session-1",
		Title:       "Focus Podcast",
		Type:        "podcast",
		Author:      func(s string) *string { return &s }("Focus Host"),
		URL:         func(s string) *string { return &s }("https://example.com/podcast"),
		Description: "A podcast on visual focus.",
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}
	rec3 := models.Recommendation{
		ID:          "rec-3",
		UserID:      user2,
		SessionID:   "session-2",
		Title:       "Other User Book",
		Type:        "book",
		Author:      func(s string) *string { return &s }("Other Author"),
		URL:         func(s string) *string { return &s }("https://example.com/other"),
		Description: "Other description.",
		CreatedAt:   time.Now(),
	}

	db.Create(&rec1)
	db.Create(&rec2)
	db.Create(&rec3)

	t.Run("Unauthorized request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/recommendations", nil)
		w := httptest.NewRecorder()

		server.GetRecommendations(w, req, api.GetRecommendationsParams{})

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Get all recommendations for user1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/recommendations", nil)
		ctx := auth.WithUserID(req.Context(), user1)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.GetRecommendations(w, req, api.GetRecommendationsParams{})

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}

		var resp api.RecommendationPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Pagination == nil || *resp.Pagination.TotalItems != 2 {
			t.Errorf("Expected 2 total items, got %v", resp.Pagination)
		}

		if len(*resp.Items) != 2 {
			t.Fatalf("Expected 2 items in response, got %d", len(*resp.Items))
		}

		// Verify order is created_at desc (rec2 then rec1)
		items := *resp.Items
		if *items[0].Id != "rec-2" || *items[1].Id != "rec-1" {
			t.Errorf("Expected order rec-2 then rec-1, got %s then %s", *items[0].Id, *items[1].Id)
		}
	})

	t.Run("Filter by type book for user1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/recommendations?type=book", nil)
		ctx := auth.WithUserID(req.Context(), user1)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		bookType := api.GetRecommendationsParamsTypeBook
		server.GetRecommendations(w, req, api.GetRecommendationsParams{
			Type: &bookType,
		})

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}

		var resp api.RecommendationPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if *resp.Pagination.TotalItems != 1 {
			t.Errorf("Expected 1 total item, got %d", *resp.Pagination.TotalItems)
		}

		items := *resp.Items
		if len(items) != 1 || *items[0].Id != "rec-1" {
			t.Errorf("Expected only rec-1, got %v", items)
		}
	})

	t.Run("Pagination limit and page", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/recommendations?page=2&limit=1", nil)
		ctx := auth.WithUserID(req.Context(), user1)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		pageVal := 2
		limitVal := 1
		server.GetRecommendations(w, req, api.GetRecommendationsParams{
			Page:  &pageVal,
			Limit: &limitVal,
		})

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Code)
		}

		var resp api.RecommendationPaginatedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if *resp.Pagination.TotalItems != 2 {
			t.Errorf("Expected 2 total items, got %d", *resp.Pagination.TotalItems)
		}
		if *resp.Pagination.TotalPages != 2 {
			t.Errorf("Expected 2 total pages, got %d", *resp.Pagination.TotalPages)
		}
		if *resp.Pagination.CurrentPage != 2 {
			t.Errorf("Expected current page 2, got %d", *resp.Pagination.CurrentPage)
		}

		items := *resp.Items
		if len(items) != 1 || *items[0].Id != "rec-1" {
			t.Errorf("Expected only rec-1 on page 2, got %v", items)
		}
	})
}
