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

// GetMemories implements api.ServerInterface
func (s *Server) GetMemories(w http.ResponseWriter, r *http.Request, params api.GetMemoriesParams) {
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

	query := database.DB.Model(&models.UserMemory{}).Where("user_id = ?", userID)

	if params.Category != nil && *params.Category != "" {
		query = query.Where("category = ?", *params.Category)
	}

	if params.Text != nil && *params.Text != "" {
		query = query.Where("content ILIKE ?", "%"+*params.Text+"%")
	}

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		s.logger.Error("Failed to count memories", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to count memories"}`, http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	totalPages := int((totalItems + int64(limit) - 1) / int64(limit))

	var memories []models.UserMemory
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&memories).Error; err != nil {
		s.logger.Error("Failed to fetch memories", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch memories"}`, http.StatusInternalServerError)
		return
	}

	apiMemories := make([]api.Memory, len(memories))
	for i, m := range memories {
		cat := api.MemoryCategory(m.Category)
		content := m.Content
		id := m.ID
		uID := m.UserID
		createdAt := m.CreatedAt

		apiMemories[i] = api.Memory{
			Id:        &id,
			UserId:    &uID,
			Category:  &cat,
			Content:   &content,
			CreatedAt: &createdAt,
		}
	}

	resp := api.MemoryPaginatedResponse{
		Items: &apiMemories,
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

// DeleteMemory implements api.ServerInterface
func (s *Server) DeleteMemory(w http.ResponseWriter, r *http.Request, memoryId string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var memory models.UserMemory
	if err := database.DB.Where("id = ? AND user_id = ?", memoryId, userID).First(&memory).Error; err != nil {
		http.Error(w, `{"error": "Memory not found"}`, http.StatusNotFound)
		return
	}

	if err := database.DB.Delete(&memory).Error; err != nil {
		s.logger.Error("Failed to delete memory", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to delete memory"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
