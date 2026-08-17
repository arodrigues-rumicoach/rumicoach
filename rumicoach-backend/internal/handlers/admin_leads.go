package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// AdminListLeads implements api.ServerInterface
func (s *Server) AdminListLeads(w http.ResponseWriter, r *http.Request, params api.AdminListLeadsParams) {
	page := 1
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	limit := 20
	offset := (page - 1) * limit

	query := database.DB.Model(&models.Lead{})
	if params.State != nil && *params.State != "" {
		query = query.Where("state = ?", *params.State)
	}
	if params.Search != nil && *params.Search != "" {
		searchTerm := "%" + *params.Search + "%"
		query = query.Where("name ILIKE ? OR email ILIKE ? OR company ILIKE ?", searchTerm, searchTerm, searchTerm)
	}
	if params.Country != nil && *params.Country != "" {
		query = query.Where("country ILIKE ?", *params.Country)
	}
	if params.Language != nil && *params.Language != "" {
		query = query.Where("language ILIKE ?", *params.Language)
	}
	if params.Size != nil && *params.Size != "" {
		query = query.Where("size = ?", *params.Size)
	}
	if params.StartDate != nil {
		query = query.Where("created_at >= ?", params.StartDate.Time)
	}
	if params.EndDate != nil {
		endOfDay := params.EndDate.Time.Add(24 * time.Hour).Add(-time.Nanosecond)
		query = query.Where("created_at <= ?", endOfDay)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		s.logger.Error("Failed to count leads", zap.Error(err))
		http.Error(w, "Failed to count leads", http.StatusInternalServerError)
		return
	}

	var leads []models.Lead
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&leads).Error; err != nil {
		s.logger.Error("Failed to list leads", zap.Error(err))
		http.Error(w, "Failed to list leads", http.StatusInternalServerError)
		return
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	totalInt := int(total)
	items := make([]api.LeadAdmin, 0)

	response := api.LeadListResponse{
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   &totalInt,
			TotalPages:   &totalPages,
		},
	}

	for _, l := range leads {
		lCopy := l
		adminLead := api.LeadAdmin{
			Id:        &lCopy.ID,
			Name:      &lCopy.Name,
			Email:     &lCopy.Email,
			Phone:     lCopy.Phone,
			Company:   &lCopy.Company,
			Country:   lCopy.Country,
			Size:      lCopy.Size,
			Message:   lCopy.Message,
			Origin:    lCopy.Origin,
			Language:  &lCopy.Language,
			State:     &lCopy.State,
			CreatedAt: &lCopy.CreatedAt,
			UpdatedAt: &lCopy.UpdatedAt,
		}
		items = append(items, adminLead)
	}
	response.Items = &items

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// AdminUpdateLead implements api.ServerInterface
func (s *Server) AdminUpdateLead(w http.ResponseWriter, r *http.Request, id types.UUID) {
	var req api.UpdateLeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	var lead models.Lead
	if err := database.DB.First(&lead, "id = ?", id.String()).Error; err != nil {
		http.Error(w, "Lead not found", http.StatusNotFound)
		return
	}

	lead.State = req.State
	if err := database.DB.Save(&lead).Error; err != nil {
		s.logger.Error("Failed to update lead", zap.Error(err))
		http.Error(w, "Failed to update lead", http.StatusInternalServerError)
		return
	}

	adminLead := api.LeadAdmin{
		Id:        &lead.ID,
		Name:      &lead.Name,
		Email:     &lead.Email,
		Phone:     lead.Phone,
		Company:   &lead.Company,
		Country:   lead.Country,
		Size:      lead.Size,
		Message:   lead.Message,
		Origin:    lead.Origin,
		Language:  &lead.Language,
		State:     &lead.State,
		CreatedAt: &lead.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adminLead)
}
