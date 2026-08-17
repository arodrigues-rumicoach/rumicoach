package handlers

import (
	"bytes"
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

func TestSubmitSessionFeedback(t *testing.T) {
	// Initialize in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db

	// Create table manually to avoid PostgreSQL timezone datatype parsing issues on SQLite
	err = db.Exec(`CREATE TABLE communication_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		duration INTEGER,
		language TEXT,
		input_tokens INTEGER,
		output_tokens INTEGER,
		total_tokens INTEGER,
		input_text_tokens INTEGER,
		output_text_tokens INTEGER,
		input_audio_tokens INTEGER,
		output_audio_tokens INTEGER,
		input_video_tokens INTEGER,
		output_video_tokens INTEGER,
		deepgram_duration REAL,
		stt_service TEXT,
		session_type TEXT,
		transcript TEXT,
		ai_notes TEXT,
		ai_evaluation REAL,
		user_evaluation REAL,
		user_feedback TEXT,
		user_session_insight TEXT,
		session_summary TEXT,
		recap TEXT,
		recap_title TEXT
	)`).Error
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	logger := zap.NewNop()
	server := NewServer(logger, nil, nil, nil)

	userID := "user-123"
	sessionID := "session-456"

	// Create a test session belonging to userID
	now := time.Now()
	testSession := models.CommunicationSession{
		ID:        sessionID,
		UserID:    userID,
		StartTime: now,
	}
	if err := db.Create(&testSession).Error; err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Create another session belonging to a different user
	otherSession := models.CommunicationSession{
		ID:        "session-other",
		UserID:    "user-other",
		StartTime: now,
	}
	if err := db.Create(&otherSession).Error; err != nil {
		t.Fatalf("failed to create other session: %v", err)
	}

	t.Run("Unauthorized request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/sessions/"+sessionID+"/feedback", nil)
		// No userID in context
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, sessionID)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Missing Session ID in URLParam", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/sessions//feedback", nil)
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, "")

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/sessions/"+sessionID+"/feedback", bytes.NewBufferString("{invalid"))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, sessionID)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Evaluation rating out of bounds (> 10.0)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"evaluation": 10.1,
			"feedback":   "Too high score",
		})
		req := httptest.NewRequest("POST", "/v1/chat/sessions/"+sessionID+"/feedback", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, sessionID)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Evaluation rating out of bounds (< 0.0)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"evaluation": -0.1,
			"feedback":   "Negative score",
		})
		req := httptest.NewRequest("POST", "/v1/chat/sessions/"+sessionID+"/feedback", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, sessionID)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Session not found", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"evaluation": 4.5,
			"feedback":   "Nice",
		})
		req := httptest.NewRequest("POST", "/v1/chat/sessions/nonexistent/feedback", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, "nonexistent")

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})

	t.Run("Forbidden session (belongs to another user)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"evaluation": 4.5,
			"feedback":   "Stealing session",
		})
		req := httptest.NewRequest("POST", "/v1/chat/sessions/session-other/feedback", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, "session-other")

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})

	t.Run("Successful submission", func(t *testing.T) {
		body, _ := json.Marshal(api.SubmitFeedbackRequest{
			Evaluation: 4.5,
			Feedback:   func(s string) *string { return &s }("Excellent session with Rumi!"),
		})
		req := httptest.NewRequest("POST", "/v1/chat/sessions/"+sessionID+"/feedback", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.SubmitSessionFeedback(w, req, sessionID)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Verify database
		var updatedSession models.CommunicationSession
		if err := db.Where("id = ?", sessionID).First(&updatedSession).Error; err != nil {
			t.Fatalf("Expected session to exist in DB: %v", err)
		}

		if updatedSession.UserEvaluation == nil || *updatedSession.UserEvaluation != 4.5 {
			t.Errorf("Expected UserEvaluation to be 4.5, got %v", updatedSession.UserEvaluation)
		}

		if updatedSession.UserFeedback == nil || *updatedSession.UserFeedback != "Excellent session with Rumi!" {
			t.Errorf("Expected UserFeedback to be 'Excellent session with Rumi!', got %v", updatedSession.UserFeedback)
		}
	})
}
