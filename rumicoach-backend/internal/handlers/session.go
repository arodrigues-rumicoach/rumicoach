package handlers

import (
	"encoding/json"
	"math"
	"net/http"

	"go.uber.org/zap"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
)

// GetSessions implements api.ServerInterface
func (s *Server) GetSessions(w http.ResponseWriter, r *http.Request, params api.GetSessionsParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	page := 1
	if params.Page != nil {
		page = *params.Page
	}
	limit := 20
	if params.Limit != nil {
		limit = *params.Limit
	}
	offset := (page - 1) * limit

	var sessions []models.CommunicationSession
	var total int64

	query := database.DB.Model(&models.CommunicationSession{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		s.logger.Error("Failed to count sessions", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to count sessions"}`, http.StatusInternalServerError)
		return
	}

	if err := query.Order("start_time desc").Offset(offset).Limit(limit).Find(&sessions).Error; err != nil {
		s.logger.Error("Failed to fetch sessions", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch sessions"}`, http.StatusInternalServerError)
		return
	}

	apiItems := make([]api.CommunicationSession, len(sessions))
	for i, session := range sessions {
		apiItems[i] = api.CommunicationSession{
			Id:                 &session.ID,
			StartTime:          &session.StartTime,
			Duration:           &session.Duration,
			SessionType:        session.SessionType,
			RecapTitle:         session.RecapTitle,
			Recap:              session.Recap,
			UserSessionInsight: session.UserSessionInsight,
			UserEvaluation: func(v *float64) *float32 {
				if v == nil {
					return nil
				}
				f := float32(*v)
				return &f
			}(session.UserEvaluation),
			UserFeedback: session.UserFeedback,
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp := api.UserSessionPaginatedResponse{
		Items: &apiItems,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(total)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
