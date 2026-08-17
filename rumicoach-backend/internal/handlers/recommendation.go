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

// GetRecommendations implements api.ServerInterface
func (s *Server) GetRecommendations(w http.ResponseWriter, r *http.Request, params api.GetRecommendationsParams) {
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

	query := database.DB.Model(&models.Recommendation{}).Where("user_id = ?", userID)
	if params.Type != nil && *params.Type != "" {
		query = query.Where("type = ?", string(*params.Type))
	}

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		s.logger.Error("Failed to count recommendations", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to count recommendations"}`, http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	totalPages := int((totalItems + int64(limit) - 1) / int64(limit))

	var recs []models.Recommendation
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&recs).Error; err != nil {
		s.logger.Error("Failed to fetch recommendations", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch recommendations"}`, http.StatusInternalServerError)
		return
	}

	apiRecs := make([]api.Recommendation, len(recs))
	for i, r := range recs {
		id := r.ID
		uID := r.UserID
		sID := r.SessionID
		title := r.Title
		desc := r.Description
		createdAt := r.CreatedAt
		recType := api.RecommendationType(r.Type)

		apiRecs[i] = api.Recommendation{
			Id:          &id,
			UserId:      &uID,
			SessionId:   &sID,
			Title:       &title,
			Type:        &recType,
			Author:      r.Author,
			Url:         r.URL,
			Description: &desc,
			CreatedAt:   &createdAt,
		}
	}

	resp := api.RecommendationPaginatedResponse{
		Items: &apiRecs,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(totalItems)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode recommendations response", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
}
