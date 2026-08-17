package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification"
	"go.uber.org/zap"
)

// PostLeads implements api.ServerInterface
func (s *Server) PostLeads(w http.ResponseWriter, r *http.Request) {
	var req api.LeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Email == "" || req.Company == "" || req.Country == "" || req.Size == "" {
		http.Error(w, `{"error": "Missing required fields"}`, http.StatusBadRequest)
		return
	}

	lead := models.Lead{
		Name:    req.Name,
		Email:   req.Email,
		Phone:   req.Phone,
		Company: req.Company,
		Country: &req.Country,
		Size:    &req.Size,
		Message: req.Message,
		Origin:  req.Origin,
		State:   "NEW",
	}

	if req.Language != nil {
		lead.Language = *req.Language
	} else {
		lead.Language = "en"
	}

	if err := database.DB.Create(&lead).Error; err != nil {
		s.logger.Error("Failed to create lead", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Send notification email to sales
	if err := notification.GlobalNotificationService.SendLeadNotificationEmail(&lead); err != nil {
		s.logger.Error("Failed to send lead notification email", zap.Error(err))
		// We don't return an error to the user if the email fails
	}

	// Send confirmation email to the user
	if err := notification.GlobalNotificationService.SendLeadConfirmationEmail(&lead); err != nil {
		s.logger.Error("Failed to send lead confirmation email", zap.Error(err))
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status": "ok"}`))
}
