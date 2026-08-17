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

// GetWheelOfLife implements api.ServerInterface
func (s *Server) GetWheelOfLife(w http.ResponseWriter, r *http.Request, params api.GetWheelOfLifeParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	page := 1
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}

	query := database.DB.Model(&models.WheelOfLifeExercise{}).Where("user_id = ?", userID)

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		s.logger.Error("Failed to count wheel of life exercises", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to count exercises"}`, http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	totalPages := int((totalItems + int64(limit) - 1) / int64(limit))

	var exercises []models.WheelOfLifeExercise
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&exercises).Error; err != nil {
		s.logger.Error("Failed to fetch wheel of life exercises", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch exercises"}`, http.StatusInternalServerError)
		return
	}

	apiExercises := make([]api.WheelOfLifeExercise, len(exercises))
	for i, ex := range exercises {
		var data []api.WheelOfLifeItem
		if err := json.Unmarshal([]byte(ex.Data), &data); err != nil {
			s.logger.Error("Failed to unmarshal wheel of life data", zap.String("exercise_id", ex.ID), zap.Error(err))
			continue
		}

		createdAt := ex.CreatedAt
		updatedAt := ex.UpdatedAt
		apiExercises[i] = api.WheelOfLifeExercise{
			Id:        &ex.ID,
			UserId:    &ex.UserID,
			SessionId: &ex.SessionID,
			Data:      &data,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}
	}

	resp := api.WheelOfLifePaginatedResponse{
		Items: &apiExercises,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(totalItems)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetWheelOfLifeId implements api.ServerInterface
func (s *Server) GetWheelOfLifeId(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var ex models.WheelOfLifeExercise
	if err := database.DB.Where("id = ? AND user_id = ?", id, userID).First(&ex).Error; err != nil {
		http.Error(w, `{"error": "Exercise not found"}`, http.StatusNotFound)
		return
	}

	var data []api.WheelOfLifeItem
	if err := json.Unmarshal([]byte(ex.Data), &data); err != nil {
		s.logger.Error("Failed to unmarshal wheel of life data", zap.String("exercise_id", ex.ID), zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	createdAt := ex.CreatedAt
	updatedAt := ex.UpdatedAt
	apiEx := api.WheelOfLifeExercise{
		Id:        &ex.ID,
		UserId:    &ex.UserID,
		SessionId: &ex.SessionID,
		Data:      &data,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiEx)
}
