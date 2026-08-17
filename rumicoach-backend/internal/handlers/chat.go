package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// SubmitSessionFeedback implements api.ServerInterface
func (s *Server) SubmitSessionFeedback(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if sessionID == "" {
		http.Error(w, `{"error": "Session ID is required"}`, http.StatusBadRequest)
		return
	}

	var req api.SubmitFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validation: evaluation rating must be decimal between 0.0 and 10.0
	if req.Evaluation < 0.0 || req.Evaluation > 10.0 {
		http.Error(w, `{"error": "Evaluation must be between 0.0 and 10.0"}`, http.StatusBadRequest)
		return
	}

	// Retrieve session
	var session models.CommunicationSession
	if err := database.DB.Where("id = ?", sessionID).First(&session).Error; err != nil {
		http.Error(w, `{"error": "Session not found"}`, http.StatusNotFound)
		return
	}

	// Authorization: ensure the session belongs to this user
	if session.UserID != userID {
		http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
		return
	}

	// Update session with user evaluation & feedback
	updates := map[string]interface{}{
		"user_evaluation": req.Evaluation,
		"user_feedback":   req.Feedback,
	}

	if err := database.DB.Model(&session).Updates(updates).Error; err != nil {
		s.logger.Error("Failed to save user feedback", zap.Error(err), zap.String("sessionID", sessionID))
		http.Error(w, `{"error": "Failed to save feedback"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"Feedback submitted successfully"}`))
}
