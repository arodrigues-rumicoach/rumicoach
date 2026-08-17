package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/quote"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// GetRandomQuote implements api.ServerInterface
func (s *Server) GetRandomQuote(w http.ResponseWriter, r *http.Request, params api.GetRandomQuoteParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var user models.User
	lang := "en-US"
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err == nil {
		if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
			lang = *user.PreferredLanguage
		}
	} else {
		s.logger.Error("User not found for quote translation", zap.Error(err), zap.String("user_id", userID))
	}

	var category *string
	if params.Category != nil {
		s := string(*params.Category)
		category = &s
	}

	quoteText, author, quoteCategory, quoteID := quote.GlobalQuoteService.GetRandomQuoteData(lang, category)

	cat := api.QuoteCategory(quoteCategory)
	resp := api.Quote{
		Id:       &quoteID,
		Quote:    &quoteText,
		Author:   author,
		Category: &cat,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode quote response")
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
}
