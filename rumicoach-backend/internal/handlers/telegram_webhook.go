package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/companion"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

type TelegramWebhookHandler struct {
	logger    *zap.Logger
	companion *companion.Service

	unknownMu        sync.Mutex
	unknownRepliedAt map[string]time.Time
}

func NewTelegramWebhookHandler(logger *zap.Logger, companionSvc *companion.Service) *TelegramWebhookHandler {
	return &TelegramWebhookHandler{
		logger:           logger,
		companion:        companionSvc,
		unknownRepliedAt: map[string]time.Time{},
	}
}

// PostWebhooksTelegramSecret implements api.ServerInterface
func (s *Server) PostWebhooksTelegramSecret(w http.ResponseWriter, r *http.Request, secret string) {
	if s.telegramWebhook != nil {
		s.telegramWebhook.Receive(w, r, secret)
	} else {
		http.Error(w, "telegram disabled", http.StatusNotFound)
	}
}

// TelegramUpdate represents the payload from Telegram Webhook.
type TelegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

type TelegramMessage struct {
	MessageID int            `json:"message_id"`
	From      *TelegramUser  `json:"from"`
	Chat      *TelegramChat  `json:"chat"`
	Date      int            `json:"date"`
	Text      string         `json:"text,omitempty"`
	Voice     *TelegramVoice `json:"voice,omitempty"`
	Audio     *TelegramAudio `json:"audio,omitempty"`
}

type TelegramUser struct {
	ID int64 `json:"id"`
}

type TelegramChat struct {
	ID int64 `json:"id"`
}

type TelegramVoice struct {
	FileID string `json:"file_id"`
}

type TelegramAudio struct {
	FileID string `json:"file_id"`
}

func (h *TelegramWebhookHandler) Receive(w http.ResponseWriter, r *http.Request, secret string) {
	if config.AppConfig.TelegramWebhookSecret != "" && secret != config.AppConfig.TelegramWebhookSecret {
		h.logger.Warn("telegram webhook: invalid secret")
		http.Error(w, "invalid secret", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		h.logger.Warn("telegram webhook: malformed payload", zap.Error(err))
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	if update.Message != nil {
		go h.processMessage(update.Message)
	}
}

func (h *TelegramWebhookHandler) processMessage(msg *TelegramMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fromID := strconv.FormatInt(msg.Chat.ID, 10)
	logger := h.logger.With(zap.String("chat_id", fromID), zap.Int("message_id", msg.MessageID))

	var integration models.Integration
	err := database.DB.
		Where("provider = ? AND external_id = ? AND status = ?", models.ChannelProviderTelegram, fromID, models.IntegrationActive).
		First(&integration).Error
	if err != nil {
		h.handleUnboundSender(ctx, msg, fromID)
		return
	}

	msgIDStr := strconv.Itoa(msg.MessageID)

	// The retention horizon is stamped here, at the only point where the row is created.
	// Deferring it to the async processing would leave a NULL — "keep forever" — on every
	// message whose processing failed, which is a retention leak in the error path.
	var owner models.User
	if err := database.DB.Select("chat_history_retention_days").
		Where("id = ?", integration.UserID).First(&owner).Error; err != nil {
		logger.Warn("could not read chat retention; message will be kept until the next change",
			zap.Error(err))
	}

	record := models.ChannelMessage{
		BindingID:         integration.ID,
		UserID:            integration.UserID,
		Provider:          models.ChannelProviderTelegram,
		Direction:         models.ChannelMessageInbound,
		ProviderMessageID: &msgIDStr,
		Status:            models.ChannelMessageReceived,
		ExpiresAt:         models.ChatMessageExpiry(time.Now(), owner.ChatHistoryRetentionDays),
	}

	if msg.Text != "" {
		record.Type = models.ChannelMessageTypeText
		body := msg.Text
		// If it's a command like /start RUMI-XXXXXX, trim the /start
		if strings.HasPrefix(body, "/start ") {
			body = strings.TrimPrefix(body, "/start ")
		}
		record.Body = &body
	} else if msg.Voice != nil {
		record.Type = models.ChannelMessageTypeAudio
		mediaID := msg.Voice.FileID
		record.MediaID = &mediaID
	} else if msg.Audio != nil {
		record.Type = models.ChannelMessageTypeAudio
		mediaID := msg.Audio.FileID
		record.MediaID = &mediaID
	} else {
		// Unsupported type
		logger.Info("telegram webhook: unsupported message type")
		h.companion.HandleUnsupportedType(ctx, &integration)
		return
	}

	// Dedupe on provider_message_id
	res := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider_message_id"}},
		DoNothing: true,
	}).Create(&record)
	if res.Error != nil {
		logger.Error("telegram webhook: failed to insert message", zap.Error(res.Error))
		return
	}
	if res.RowsAffected == 0 {
		logger.Info("telegram webhook: duplicate delivery dropped")
		return
	}

	h.companion.HandleInbound(ctx, &integration, &record)
}

func (h *TelegramWebhookHandler) handleUnboundSender(ctx context.Context, msg *TelegramMessage, fromID string) {
	if msg.Text != "" {
		text := msg.Text
		// Support `/start RUMI-XXXXXX` from deep link
		if strings.HasPrefix(text, "/start ") {
			text = strings.TrimPrefix(text, "/start ")
		}
		if match := linkCodePattern.FindString(strings.ToUpper(text)); match != "" {
			if h.companion.HandleLinkCode(ctx, models.ChannelProviderTelegram, fromID, match) {
				return
			}
			h.logger.Info("telegram webhook: link code not recognized or expired")
		}
	}

	h.unknownMu.Lock()
	last, seen := h.unknownRepliedAt[fromID]
	if seen && time.Since(last) < 24*time.Hour {
		h.unknownMu.Unlock()
		return
	}
	h.unknownRepliedAt[fromID] = time.Now()
	for id, t := range h.unknownRepliedAt {
		if time.Since(t) > 48*time.Hour {
			delete(h.unknownRepliedAt, id)
		}
	}
	h.unknownMu.Unlock()

	h.companion.HandleUnknownSender(ctx, models.ChannelProviderTelegram, fromID)
}
