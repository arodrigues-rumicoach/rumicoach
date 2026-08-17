package handlers

import (
	"net/http"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

type TwilioWebhookHandler struct {
	logger *zap.Logger
}

func NewTwilioWebhookHandler(logger *zap.Logger) *TwilioWebhookHandler {
	return &TwilioWebhookHandler{logger: logger}
}

// PostWebhooksTwilioSms implements api.ServerInterface
func (s *Server) PostWebhooksTwilioSms(w http.ResponseWriter, r *http.Request) {
	s.twilioWebhook.Receive(w, r)
}

func (h *TwilioWebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		// A malformed webhook body is the sender's problem, not ours; we answer 400 and move on.
		h.logger.Info("twilio webhook: could not parse form", zap.Error(err))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	from := r.FormValue("From")
	body := r.FormValue("Body")
	to := r.FormValue("To")

	// The message body is stored in twilio_logs, where it can be governed; it is not
	// logged, where it cannot. Phone numbers are identifiers in their own right, so only
	// the destination — ours — is named.
	h.logger.Info("Received Twilio SMS",
		zap.String("to", to),
		zap.Int("body_chars", len(body)),
	)

	logEntry := models.TwilioLog{
		From: from,
		To:   to,
		Body: body,
	}

	if err := database.DB.Create(&logEntry).Error; err != nil {
		h.logger.Error("failed to save twilio log", zap.Error(err))
	}

	// Acknowledge Twilio
	w.WriteHeader(http.StatusOK)
}
