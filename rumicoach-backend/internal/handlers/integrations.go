package handlers

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const linkCodeTTL = 15 * time.Minute

// linkCodeAlphabet is Crockford base32 (no I, L, O, U) — unambiguous when the
// user reads or types the code, and a subset of the webhook's RUMI-[A-Z0-9]{6}
// matcher.
const linkCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// GetChannels lists the user's messaging channel integrations.
func (s *Server) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var integrations []models.Integration
	if err := database.DB.
		Where("user_id = ? AND status <> ?", userID, models.IntegrationRevoked).
		Order("created_at asc").
		Find(&integrations).Error; err != nil {
		s.logger.Error("Failed to list channel integrations", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	resp := make([]api.Integration, 0, len(integrations))
	for _, b := range integrations {
		resp = append(resp, toAPIIntegration(b))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// LinkWhatsAppIntegration issues a fresh one-time link code and wa.me deep link.
// Any previous integration for the provider is replaced.
func (s *Server) LinkWhatsAppIntegration(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if !config.AppConfig.WhatsAppEnabled || config.AppConfig.WhatsAppBusinessNumber == "" {
		http.Error(w, `{"error": "WhatsApp channel is not available"}`, http.StatusServiceUnavailable)
		return
	}

	// See the note in LinkTelegramIntegration: issuing a link code tears down whatever
	// binding exists, so refuse when one is already working.
	var active models.Integration
	if err := database.DB.
		Where("user_id = ? AND provider = ? AND status = ?", userID, models.ChannelProviderWhatsApp, models.IntegrationActive).
		First(&active).Error; err == nil {
		apierror.Write(w, http.StatusConflict, apierror.CodeIntegrationAlreadyActive,
			"This channel is already connected. Disconnect it before linking again.")
		return
	}

	code, err := generateLinkCode()
	if err != nil {
		s.logger.Error("Failed to generate link code", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().Add(linkCodeTTL)

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Linking again replaces any existing integration for this provider (the
		// user may be switching numbers).
		if err := tx.Model(&models.Integration{}).
			Where("user_id = ? AND provider = ? AND status <> ?", userID, models.ChannelProviderWhatsApp, models.IntegrationRevoked).
			Updates(map[string]any{"status": models.IntegrationRevoked, "external_id": gorm.Expr("NULL"), "link_code": nil}).Error; err != nil {
			return err
		}
		integration := models.Integration{
			UserID:            userID,
			Provider:          models.ChannelProviderWhatsApp,
			Status:            models.IntegrationPending,
			LinkCode:          &code,
			LinkCodeExpiresAt: &expiresAt,
			ReplyMode:         models.ChannelReplyModeText,
		}
		return tx.Create(&integration).Error
	})
	if err != nil {
		s.logger.Error("Failed to create channel integration", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	waLink := fmt.Sprintf("https://wa.me/%s?text=%s", config.AppConfig.WhatsAppBusinessNumber, url.QueryEscape(code))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(api.ChannelLinkResponse{Code: code, WaLink: waLink, ExpiresAt: expiresAt})
}

// LinkTelegramIntegration issues a fresh one-time link code and t.me deep link.
// Any previous integration for the provider is replaced.
func (s *Server) LinkTelegramIntegration(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if !config.AppConfig.TelegramEnabled || config.AppConfig.TelegramBotUsername == "" {
		http.Error(w, `{"error": "Telegram channel is not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Issuing a link code tears down whatever binding exists, so refuse when one is
	// already working. This endpoint used to revoke an ACTIVE integration on every call,
	// which meant any client bug, double tap, retry, or deep link silently disconnected
	// a user who was happily connected — QA hit exactly that by opening the integrations
	// screen before the list had loaded, so the app could not tell there was anything to
	// preserve. Re-linking is a deliberate act: disconnect first (DELETE), then link.
	var active models.Integration
	if err := database.DB.
		Where("user_id = ? AND provider = ? AND status = ?", userID, models.ChannelProviderTelegram, models.IntegrationActive).
		First(&active).Error; err == nil {
		apierror.Write(w, http.StatusConflict, apierror.CodeIntegrationAlreadyActive,
			"This channel is already connected. Disconnect it before linking again.")
		return
	}

	code, err := generateLinkCode()
	if err != nil {
		s.logger.Error("Failed to generate link code", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
	expiresAt := time.Now().Add(linkCodeTTL)

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Integration{}).
			Where("user_id = ? AND provider = ? AND status <> ?", userID, models.ChannelProviderTelegram, models.IntegrationRevoked).
			Updates(map[string]any{"status": models.IntegrationRevoked, "external_id": gorm.Expr("NULL"), "link_code": nil}).Error; err != nil {
			return err
		}
		integration := models.Integration{
			UserID:            userID,
			Provider:          models.ChannelProviderTelegram,
			Status:            models.IntegrationPending,
			LinkCode:          &code,
			LinkCodeExpiresAt: &expiresAt,
			ReplyMode:         models.ChannelReplyModeText,
		}
		return tx.Create(&integration).Error
	})
	if err != nil {
		s.logger.Error("Failed to create channel integration", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// A configured "@name" would yield https://t.me/@name?start=…, which
	// resolves to the chat but drops the start payload — the bot then receives a
	// bare /start and cannot tell which account is linking.
	botUsername := strings.TrimPrefix(config.AppConfig.TelegramBotUsername, "@")
	waLink := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, url.QueryEscape(code))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(api.ChannelLinkResponse{Code: code, WaLink: waLink, ExpiresAt: expiresAt})
}

// findLiveIntegration loads the user's current binding for `ref`, which is
// either an integration id or a provider name.
//
// Revoked rows are excluded, matching GetIntegrations and DeleteIntegration:
// linking replaces rather than updates, so a user who has re-linked a provider
// keeps older revoked rows alongside the live one. Without the filter — and
// with GORM's default `First` ordering by primary key, which for these text
// UUIDs is arbitrary — a lookup by provider name could return a dead binding.
// Newest first so the live row wins even if duplicates ever slip through.
func findLiveIntegration(userID, ref string) (models.Integration, error) {
	var integration models.Integration
	err := database.DB.
		Where("(id = ? OR provider = ?) AND user_id = ? AND status <> ?",
			ref, ref, userID, models.IntegrationRevoked).
		Order("created_at DESC").
		First(&integration).Error
	return integration, err
}

// GetIntegration returns a single integration by ID or provider.
func (s *Server) GetIntegration(w http.ResponseWriter, r *http.Request, integrationId string) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	integration, err := findLiveIntegration(userID, integrationId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, `{"error": "Channel integration not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to load channel integration", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toAPIIntegration(integration))
}

// UpdateIntegration changes integration settings (reply mode).
func (s *Server) UpdateIntegration(w http.ResponseWriter, r *http.Request, integrationId string) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req api.ChannelUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.ReplyMode == nil {
		http.Error(w, `{"error": "replyMode is required"}`, http.StatusBadRequest)
		return
	}
	replyMode := string(*req.ReplyMode)
	if replyMode != models.ChannelReplyModeText && replyMode != models.ChannelReplyModeAudio && replyMode != models.ChannelReplyModeAuto {
		http.Error(w, `{"error": "replyMode must be 'text', 'audio', or 'auto'"}`, http.StatusBadRequest)
		return
	}

	integration, err := findLiveIntegration(userID, integrationId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, `{"error": "Channel integration not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to load channel integration", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
	integration.ReplyMode = replyMode
	if err := database.DB.Model(&integration).Update("reply_mode", replyMode).Error; err != nil {
		s.logger.Error("Failed to update channel integration", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toAPIIntegration(integration))
}

// DeleteChannel revokes a integration (frees the number for re-linking).
func (s *Server) DeleteIntegration(w http.ResponseWriter, r *http.Request, integrationId string) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	res := database.DB.Model(&models.Integration{}).
		Where("(id = ? OR provider = ?) AND user_id = ? AND status <> ?", integrationId, integrationId, userID, models.IntegrationRevoked).
		Updates(map[string]any{"status": models.IntegrationRevoked, "external_id": gorm.Expr("NULL"), "link_code": nil})
	if res.Error != nil {
		s.logger.Error("Failed to revoke channel integration", zap.Error(res.Error))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, `{"error": "Channel integration not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toAPIIntegration(i models.Integration) api.Integration {
	out := api.Integration{
		Id:        i.ID,
		Provider:  i.Provider,
		Status:    i.Status,
		ReplyMode: api.IntegrationReplyMode(i.ReplyMode),
		CreatedAt: i.CreatedAt,
	}
	if i.ExternalID != nil {
		masked := maskExternalID(*i.ExternalID)
		out.MaskedExternalId = &masked
	}
	return out
}

// maskExternalID hides the middle of a phone number/chat id: "3519•••••78".
func maskExternalID(id string) string {
	runes := []rune(id)
	if len(runes) <= 6 {
		return "•••"
	}
	keepFront, keepBack := 4, 2
	masked := make([]rune, 0, len(runes))
	masked = append(masked, runes[:keepFront]...)
	for range runes[keepFront : len(runes)-keepBack] {
		masked = append(masked, '•')
	}
	masked = append(masked, runes[len(runes)-keepBack:]...)
	return string(masked)
}

// generateLinkCode returns "RUMI-XXXXXX" using crypto/rand over the Crockford
// base32 alphabet.
func generateLinkCode() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := make([]byte, 6)
	for i, b := range buf {
		code[i] = linkCodeAlphabet[int(b)%len(linkCodeAlphabet)]
	}
	return "RUMI-" + string(code), nil
}
