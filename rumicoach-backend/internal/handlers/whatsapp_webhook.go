package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/companion"
	"github.com/rumi/rumi-be/internal/services/messaging/whatsapp"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

// linkCodePattern matches the app-issued channel link code sent as the first
// message (wa.me deep link pre-fills it).
var linkCodePattern = regexp.MustCompile(`\bRUMI-([A-Z0-9]{6})\b`)

// WhatsAppWebhookHandler receives Meta's webhook traffic for this region's
// WhatsApp Business number. The route is registered outside /v1 (no user JWT);
// authenticity comes from the X-Hub-Signature-256 HMAC over the raw body.
type WhatsAppWebhookHandler struct {
	logger    *zap.Logger
	companion *companion.Service

	// unknownRepliedAt rate-limits the "link your account" reply to unknown
	// numbers: at most one per number per 24h (best effort, per replica).
	unknownMu        sync.Mutex
	unknownRepliedAt map[string]time.Time
}

func NewWhatsAppWebhookHandler(logger *zap.Logger, companionSvc *companion.Service) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{
		logger:           logger,
		companion:        companionSvc,
		unknownRepliedAt: map[string]time.Time{},
	}
}

// GetWebhooksWhatsapp implements api.ServerInterface
func (s *Server) GetWebhooksWhatsapp(w http.ResponseWriter, r *http.Request) {
	if s.whatsappWebhook != nil {
		s.whatsappWebhook.Verify(w, r)
	} else {
		http.Error(w, "whatsapp disabled", http.StatusNotFound)
	}
}

// PostWebhooksWhatsapp implements api.ServerInterface
func (s *Server) PostWebhooksWhatsapp(w http.ResponseWriter, r *http.Request) {
	if s.whatsappWebhook != nil {
		s.whatsappWebhook.Receive(w, r)
	} else {
		http.Error(w, "whatsapp disabled", http.StatusNotFound)
	}
}

// Verify answers Meta's subscription handshake (hub.challenge echo).
func (h *WhatsAppWebhookHandler) Verify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("hub.mode") == "subscribe" && q.Get("hub.verify_token") == config.AppConfig.WhatsAppVerifyToken && config.AppConfig.WhatsAppVerifyToken != "" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(q.Get("hub.challenge")))
		return
	}
	http.Error(w, "verification failed", http.StatusForbidden)
}

// Receive validates the signature, acks immediately (Meta retries slow or
// non-200 deliveries), and processes the events asynchronously.
func (h *WhatsAppWebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !whatsapp.VerifySignature(config.AppConfig.WhatsAppAppSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		h.logger.Warn("whatsapp webhook: invalid signature")
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}

	var envelope whatsapp.WebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		h.logger.Warn("whatsapp webhook: malformed payload", zap.Error(err))
		// Still 200: Meta would otherwise retry a payload we can never parse.
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	go h.processEnvelope(envelope)
}

func (h *WhatsAppWebhookHandler) processEnvelope(envelope whatsapp.WebhookEnvelope) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	for _, entry := range envelope.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			// Delivery statuses are acked and dropped in v1.
			for _, msg := range change.Value.Messages {
				h.processMessage(ctx, msg)
			}
		}
	}
}

func (h *WhatsAppWebhookHandler) processMessage(ctx context.Context, msg whatsapp.Message) {
	logger := h.logger.With(zap.String("wamid", msg.ID), zap.String("type", msg.Type))

	var integration models.Integration
	err := database.DB.
		Where("provider = ? AND external_id = ? AND status = ?", models.ChannelProviderWhatsApp, msg.From, models.IntegrationActive).
		First(&integration).Error
	if err != nil {
		h.handleUnboundSender(ctx, msg)
		return
	}

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
		Provider:          models.ChannelProviderWhatsApp,
		Direction:         models.ChannelMessageInbound,
		ProviderMessageID: &msg.ID,
		Status:            models.ChannelMessageReceived,
		ExpiresAt:         models.ChatMessageExpiry(time.Now(), owner.ChatHistoryRetentionDays),
	}
	switch msg.Type {
	case "text":
		record.Type = models.ChannelMessageTypeText
		if msg.Text != nil {
			body := msg.Text.Body
			record.Body = &body
		}
	case "audio":
		record.Type = models.ChannelMessageTypeAudio
		if msg.Audio != nil {
			mediaID := msg.Audio.ID
			record.MediaID = &mediaID
		}
	default:
		// Unsupported type (image, sticker, location, …): polite pushback via
		// the companion so the user isn't left on read.
		logger.Info("whatsapp webhook: unsupported message type")
		h.companion.HandleUnsupportedType(ctx, &integration)
		return
	}

	// Dedupe on the provider message id — Meta redelivers webhooks.
	res := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider_message_id"}},
		DoNothing: true,
	}).Create(&record)
	if res.Error != nil {
		logger.Error("whatsapp webhook: failed to insert message", zap.Error(res.Error))
		return
	}
	if res.RowsAffected == 0 {
		logger.Info("whatsapp webhook: duplicate delivery dropped")
		return
	}

	h.companion.HandleInbound(ctx, &integration, &record)
}

// handleUnboundSender links a pending integration when the message carries a link
// code, otherwise sends the one-time "get the app" reply.
func (h *WhatsAppWebhookHandler) handleUnboundSender(ctx context.Context, msg whatsapp.Message) {
	if msg.Type == "text" && msg.Text != nil {
		if match := linkCodePattern.FindString(strings.ToUpper(msg.Text.Body)); match != "" {
			if h.companion.HandleLinkCode(ctx, models.ChannelProviderWhatsApp, msg.From, match) {
				return
			}
			h.logger.Info("whatsapp webhook: link code not recognized or expired")
		}
	}

	h.unknownMu.Lock()
	last, seen := h.unknownRepliedAt[msg.From]
	if seen && time.Since(last) < 24*time.Hour {
		h.unknownMu.Unlock()
		return
	}
	h.unknownRepliedAt[msg.From] = time.Now()
	// Opportunistic sweep so the map cannot grow unbounded.
	for number, t := range h.unknownRepliedAt {
		if time.Since(t) > 48*time.Hour {
			delete(h.unknownRepliedAt, number)
		}
	}
	h.unknownMu.Unlock()

	h.companion.HandleUnknownSender(ctx, models.ChannelProviderWhatsApp, msg.From)
}
