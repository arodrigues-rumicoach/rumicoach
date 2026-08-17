package companion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/i18n"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/aiusage"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// historyWindow / historyLimit bound the conversation context replayed to
	// Gemini on each turn (the pipeline is stateless per turn).
	historyWindow = 72 * time.Hour
	historyLimit  = 30
	// maxToolIterations caps the function-calling loop per turn.
	maxToolIterations = 3
	// serviceWindow is WhatsApp's customer-service window for free-form sends.
	serviceWindow = 24 * time.Hour
)

// unknownSenderMessage is sent to numbers with no integration. English by design:
// we know nothing about an unlinked sender, including their language.
const unknownSenderMessage = "Hi! This is Rumi, your AI life coach. I don't recognize this number yet. To chat with me here, open the Rumi app, go to Settings, and tap \"Connect WhatsApp\" — it links this number to your account in one tap."

// Service orchestrates companion conversations over messaging channels.
type Service struct {
	logger    *zap.Logger
	localizer *i18n.Localizer
}

func NewService(logger *zap.Logger, localizer *i18n.Localizer) *Service {
	return &Service{logger: logger, localizer: localizer}
}

// HandleInbound processes one deduplicated inbound ChannelMessage end-to-end:
// transcription (audio), Gemini chat with tools, and the reply send. Called
// asynchronously from the webhook handler.
func (s *Service) HandleInbound(ctx context.Context, integration *models.Integration, msg *models.ChannelMessage) {
	logger := s.logger.With(zap.String("userID", integration.UserID), zap.String("messageID", msg.ID))

	now := time.Now()
	integration.LastInboundAt = &now
	// Roll the daily counter in the same write. It is stored on the binding rather than
	// counted from channel_messages so that clearing the chat history cannot reset the
	// cap — which would otherwise make "delete my chat" a way to buy more messages.
	integration.RollDailyInbound(now)
	if err := database.DB.Model(integration).Updates(map[string]interface{}{
		"last_inbound_at":     now,
		"daily_inbound_count": integration.DailyInboundCount,
		"daily_inbound_date":  integration.DailyInboundDate,
	}).Error; err != nil {
		logger.Warn("companion: failed to update inbound state", zap.Error(err))
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", integration.UserID).Error; err != nil {
		logger.Error("companion: failed to load user for integration", zap.Error(err))
		s.markMessage(msg, models.ChannelMessageFailed)
		return
	}

	channel, err := messaging.Get(integration.Provider)
	if err != nil {
		logger.Error("companion: no channel for provider", zap.Error(err))
		s.markMessage(msg, models.ChannelMessageFailed)
		return
	}

	// Daily cap: today's inbound messages (UTC), current one included.
	if capped, notify := s.dailyCapState(integration); capped {
		s.markMessage(msg, models.ChannelMessageProcessed)
		if notify {
			s.generateAndSendSystemReply(ctx, channel, integration, &user,
				"SYSTEM: The user has reached their daily message limit on this channel. In ONE short, warm message in the user's language, let them know you need to pause this chat until tomorrow, and that they can always continue in the Rumi app.",
				"You've reached today's chat limit here — let's pick this up tomorrow, or continue anytime in the Rumi app.")
		}
		return
	}

	// What this inbound message costs us, kept PER MODEL rather than summed:
	// transcription and the chat loop run on different models at different prices, so
	// merging them here would leave the cost ledger unable to price either. It lands
	// in the durable record whatever way the turn ends.
	var spend Spend

	// Voice notes: fetch the media and transcribe it so the turn (and stored
	// history) is text. Gemini does the STT — no separate vendor.
	if msg.Type == models.ChannelMessageTypeAudio {
		transcript, audioSeconds, sttUsage, err := s.transcribeInbound(ctx, channel, msg)
		spend.STT.Add(sttUsage)
		// What the user is charged for their side: a spoken message is billed by its
		// length, a typed one costs nothing (the flat fee is on the reply).
		spend.InboundAudioSeconds = audioSeconds
		if err != nil {
			logger.Error("companion: transcription failed", zap.Error(err))
			s.markMessage(msg, models.ChannelMessageFailed)
			s.recordUsage(integration, &msg.ID, spend)
			s.generateAndSendSystemReply(ctx, channel, integration, &user,
				"SYSTEM: You could not hear the user's last voice note (technical problem on your side). In ONE short message in the user's language, apologize briefly and ask them to send it again or type it.",
				"Sorry, I couldn't listen to that voice note. Could you send it again, or type it?")
			return
		}
		msg.Body = &transcript
		if err := database.DB.Model(msg).Update("body", transcript).Error; err != nil {
			logger.Warn("companion: failed to persist transcript", zap.Error(err))
		}
	}

	if msg.Body == nil || strings.TrimSpace(*msg.Body) == "" {
		s.markMessage(msg, models.ChannelMessageProcessed)
		s.recordUsage(integration, &msg.ID, spend)
		return
	}

	replyText, genUsage, err := s.generateReply(ctx, &user, integration, msg)
	spend.Chat.Add(genUsage)
	if err != nil {
		logger.Error("companion: reply generation failed", zap.Error(err))
		s.markMessage(msg, models.ChannelMessageFailed)
		s.recordUsage(integration, &msg.ID, spend)
		return
	}
	msg.Status = models.ChannelMessageProcessed
	if err := database.DB.Save(msg).Error; err != nil {
		logger.Warn("companion: failed to persist processed message", zap.Error(err))
	}
	s.recordUsage(integration, &msg.ID, spend)

	if replyText == "" {
		logger.Warn("companion: model produced empty reply")
		return
	}
	s.deliverReply(ctx, channel, integration, &user, replyText, msg, Spend{})
}

// generateReply runs the stateless chat turn: system prompt + history +
// current message, with a bounded function-calling loop.
func (s *Service) generateReply(ctx context.Context, user *models.User, integration *models.Integration, current *models.ChannelMessage) (string, Usage, error) {
	systemPrompt := BuildSystemPrompt(user, s.logger)
	contents := s.loadHistory(integration, current.ID)
	contents = append(contents, Content{Role: "user", Parts: []Part{{Text: *current.Body}}})

	return s.runChatTurn(ctx, user.ID, systemPrompt, contents)
}

// runChatTurn calls Gemini with the companion tool set, executing function
// calls and re-calling until the model answers in text (bounded). The returned
// Usage sums EVERY call the loop made — including on the error paths, where the
// tokens were spent all the same and still have to reach the usage record.
func (s *Service) runChatTurn(ctx context.Context, userID, systemPrompt string, contents []Content) (string, Usage, error) {
	var usage Usage
	for i := 0; i <= maxToolIterations; i++ {
		req := Request{
			SystemInstruction: &Content{Parts: []Part{{Text: systemPrompt}}},
			Contents:          contents,
			Tools:             companionTools,
		}
		resp, err := callGemini(ctx, chatModel(), req)
		if err != nil {
			return "", usage, err
		}
		usage.AddResponse(resp)
		candidate, err := firstCandidate(resp)
		if err != nil {
			return "", usage, err
		}

		var text string
		var calls []*FunctionCall
		for _, part := range candidate.Parts {
			if part.FunctionCall != nil {
				calls = append(calls, part.FunctionCall)
			}
			if part.Text != "" {
				text += part.Text
			}
		}
		if len(calls) == 0 {
			return strings.TrimSpace(text), usage, nil
		}

		// Execute the calls and loop with their responses appended.
		contents = append(contents, candidate)
		responseParts := make([]Part, 0, len(calls))
		for _, call := range calls {
			result := s.executeTool(ctx, userID, call)
			responseParts = append(responseParts, Part{FunctionResponse: &FunctionResponse{Name: call.Name, Response: result}})
		}
		contents = append(contents, Content{Role: "user", Parts: responseParts})

		// The model often answers text alongside tool calls; accept it rather
		// than burning another round-trip.
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), usage, nil
		}
	}
	return "", usage, fmt.Errorf("model did not produce a text reply within %d tool iterations", maxToolIterations)
}

// loadHistory returns the recent conversation as Gemini contents, oldest first.
func (s *Service) loadHistory(integration *models.Integration, excludeID string) []Content {
	var rows []models.ChannelMessage
	since := time.Now().Add(-historyWindow)
	if err := database.DB.
		Where("binding_id = ? AND id <> ? AND created_at >= ? AND status <> ? AND body IS NOT NULL", integration.ID, excludeID, since, models.ChannelMessageFailed).
		Order("created_at desc").
		Limit(historyLimit).
		Find(&rows).Error; err != nil {
		s.logger.Warn("companion: failed to load history", zap.Error(err))
		return nil
	}

	contents := make([]Content, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- { // reverse to chronological order
		row := rows[i]
		role := "user"
		if row.Direction == models.ChannelMessageOutbound {
			role = "model"
		}
		contents = append(contents, Content{Role: role, Parts: []Part{{Text: *row.Body}}})
	}
	return contents
}

// deliverReply sends the reply respecting the integration's reply mode (voice
// notes fall back to text on any TTS/transcode failure) and records the
// outbound ChannelMessage, reporting whether the send reached the provider.
// genUsage is the token spend that has no inbound turn
// to be recorded on (system/proactive message generation) — replies to a user
// message pass a zero Usage because HandleInbound already recorded the turn on
// the inbound row. TTS tokens for voice notes are added here either way.
func (s *Service) deliverReply(ctx context.Context, channel messaging.Channel, integration *models.Integration, user *models.User, text string, currentMsg *models.ChannelMessage, spend Spend) bool {
	to := messaging.Address{Provider: integration.Provider, ExternalID: deref(integration.ExternalID)}

	msgType := models.ChannelMessageTypeText
	var providerMsgID string
	var err error

	replyMode := integration.ReplyMode
	if replyMode == models.ChannelReplyModeAuto && currentMsg != nil {
		if currentMsg.Type == models.ChannelMessageTypeAudio {
			replyMode = models.ChannelReplyModeAudio
		} else {
			replyMode = models.ChannelReplyModeText
		}
	}

	if replyMode == models.ChannelReplyModeAudio {
		var ttsUsage Usage
		var spokenSeconds int64
		providerMsgID, spokenSeconds, ttsUsage, err = s.sendVoiceReply(ctx, channel, to, user, text)
		spend.OutboundAudioSeconds = spokenSeconds
		// TTS tokens are recorded even on failure: a synthesis that died after the
		// model call still cost them, and the text fallback below sends anyway. Kept
		// in their own group — synthesis is the most expensive model we call.
		spend.TTS.Add(ttsUsage)
		if err != nil {
			s.logger.Warn("companion: voice reply failed, falling back to text", zap.Error(err))
		} else {
			msgType = models.ChannelMessageTypeAudio
		}
	}
	if msgType == models.ChannelMessageTypeText {
		providerMsgID, err = channel.SendText(ctx, to, text)
		if err != nil {
			s.logger.Error("companion: failed to send reply", zap.Error(err))
			s.recordOutbound(integration, msgType, text, nil, models.ChannelMessageFailed, user.ChatHistoryRetentionDays, spend)
			return false
		}
	}
	s.recordOutbound(integration, msgType, text, &providerMsgID, models.ChannelMessageSent, user.ChatHistoryRetentionDays, spend)

	// Charge the reply — and only a reply. currentMsg is the user's message being
	// answered; it is nil for everything we send on our own initiative (proactive
	// nudges, re-engagement templates, the daily-cap and transcription-failure
	// notices), and billing someone for a message they did not ask for is
	// indefensible. Charged after the send succeeded, so a failure above costs
	// nothing: the user got no reply.
	//
	// Log-and-continue, like the session debit: a balance write must never be the
	// reason a conversation breaks. A duplicate means the same inbound message was
	// answered twice and is already charged — not an error.
	if currentMsg != nil {
		if _, err := balance.DebitMessage(ctx, integration.UserID, currentMsg.ID, spend.ChargeSeconds(msgType)); err != nil &&
			!errors.Is(err, balance.ErrDuplicateReference) {
			s.logger.Error("companion: failed to debit reply", zap.Error(err),
				zap.String("userID", integration.UserID), zap.String("inboundMessageID", currentMsg.ID))
		}
	}
	return true
}

// HandleLinkCode activates a pending integration for the sender's phone number and
// has Rumi greet them. Returns false when the code is unknown or expired.
func (s *Service) HandleLinkCode(ctx context.Context, provider, externalID, code string) bool {
	var integration models.Integration
	err := database.DB.
		Where("provider = ? AND link_code = ? AND status = ? AND link_code_expires_at > ?", provider, code, models.IntegrationPending, time.Now()).
		First(&integration).Error
	if err != nil {
		return false
	}

	// Steal the number from any previous integration (user relinked from a new account).
	database.DB.Model(&models.Integration{}).
		Where("provider = ? AND external_id = ? AND id <> ?", provider, externalID, integration.ID).
		Updates(map[string]any{"status": models.IntegrationRevoked, "external_id": gorm.Expr("NULL")})

	updates := map[string]any{
		"external_id":          externalID,
		"status":               models.IntegrationActive,
		"link_code":            nil,
		"link_code_expires_at": nil,
		"last_inbound_at":      time.Now(),
	}
	if err := database.DB.Model(&integration).Updates(updates).Error; err != nil {
		s.logger.Error("companion: failed to activate integration", zap.Error(err))
		return false
	}
	integration.ExternalID = &externalID
	s.logger.Info("companion: channel linked",
		zap.String("userID", integration.UserID), zap.String("provider", provider))

	var user models.User
	if err := database.DB.First(&user, "id = ?", integration.UserID).Error; err != nil {
		s.logger.Error("companion: failed to load user after linking", zap.Error(err))
		return true
	}
	channel, err := messaging.Get(provider)
	if err != nil {
		return true
	}
	lang := "en"
	if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
		lang = *user.PreferredLanguage
	}
	msgText := ""
	if s.localizer != nil {
		msgText = s.localizer.Get(lang, fmt.Sprintf("%s.integration_success", provider), nil)
	}
	if msgText == "" {
		providerName := "WhatsApp"
		if provider == "telegram" {
			providerName = "Telegram"
		}
		msgText = fmt.Sprintf("Hey! Your %s is now connected! 👋 Rumi is here to help you achieve your goals, manage your tasks, and support you whenever you need. Feel free to message me anytime—text or voice notes, whatever feels natural!", providerName)
	}

	if _, err := channel.SendText(ctx, messaging.Address{Provider: provider, ExternalID: externalID}, msgText); err != nil {
		s.logger.Warn("companion: failed to send integration success message", zap.Error(err))
	}
	return true
}

// HandleUnsupportedType tells the user (in their language) that only text and
// voice notes work on this channel.
func (s *Service) HandleUnsupportedType(ctx context.Context, integration *models.Integration) {
	channel, err := messaging.Get(integration.Provider)
	if err != nil {
		return
	}
	var user models.User
	if err := database.DB.First(&user, "id = ?", integration.UserID).Error; err != nil {
		s.logger.Warn("companion: failed to load user for unsupported-type reply", zap.Error(err))
		return
	}
	s.generateAndSendSystemReply(ctx, channel, integration, &user,
		"SYSTEM: The user just sent a message type you cannot receive on this channel (an image, sticker, video, or similar). In ONE short, friendly message in the user's language, let them know you can only read text and voice notes here.",
		"I can only read text and voice notes here — could you send that as one of those?")
}

// HandleUnknownSender replies to a number with no integration, pointing at the app.
func (s *Service) HandleUnknownSender(ctx context.Context, provider, externalID string) {
	channel, err := messaging.Get(provider)
	if err != nil {
		return
	}

	// Attempt to find the user by phone number to respond in their language
	var user models.User
	if provider == models.ChannelProviderWhatsApp {
		err := database.DB.Where("phone_number = ? OR phone_number = ?", externalID, "+"+externalID).First(&user).Error
		if err == nil {
			lang := "en"
			if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
				lang = *user.PreferredLanguage
			}
			msgText := ""
			if s.localizer != nil {
				msgText = s.localizer.Get(lang, "whatsapp.invalid_link_code", nil)
			}
			if msgText != "" {
				if _, err := channel.SendText(ctx, messaging.Address{Provider: provider, ExternalID: externalID}, msgText); err != nil {
					s.logger.Warn("companion: failed to reply to unknown sender in their language", zap.Error(err))
				}
				return
			}
		}
	}

	if _, err := channel.SendText(ctx, messaging.Address{Provider: provider, ExternalID: externalID}, unknownSenderMessage); err != nil {
		s.logger.Warn("companion: failed to reply to unknown sender", zap.Error(err))
	}
}

// generateAndSendSystemReply produces a one-off Rumi message from a system
// directive (welcome, cap notice, transcription apology, proactive follow-up)
// in the user's language, with a static English fallback, and delivers +
// records it. It returns the text it tried to deliver and whether the send
// reached the provider, so proactive callers can log what was actually said.
func (s *Service) generateAndSendSystemReply(ctx context.Context, channel messaging.Channel, integration *models.Integration, user *models.User, directive, fallback string) (string, bool) {
	systemPrompt := BuildSystemPrompt(user, s.logger)
	contents := s.loadHistory(integration, "")
	contents = append(contents, Content{Role: "user", Parts: []Part{{Text: directive}}})

	text, usage, err := s.runChatTurn(ctx, user.ID, systemPrompt, contents)
	if err != nil || text == "" {
		if err != nil {
			s.logger.Warn("companion: system reply generation failed, using fallback", zap.Error(err))
		}
		text = fallback
	}
	// These turns have no inbound message, so the generation tokens ride on the
	// outbound usage record (fallback or not — the model call already happened).
	sent := s.deliverReply(ctx, channel, integration, user, text, nil, Spend{Chat: usage})
	return text, sent
}

// dailyCapState reports whether the user is over today's inbound-message cap and whether
// the cap notice should be sent (exactly once, on the message that crosses the line).
// The count comes from the binding, already rolled for this message by HandleInbound, so
// it survives a chat purge; it used to be a COUNT over channel_messages on every message.
func (s *Service) dailyCapState(integration *models.Integration) (capped, notify bool) {
	cap := config.AppConfig.CompanionDailyMessageCap
	if cap <= 0 {
		return false, false
	}
	count := integration.DailyInboundCount
	if count <= cap {
		return false, false
	}
	return true, count == cap+1
}

// recordOutbound stores a message we sent and stamps the binding. retentionDays comes from
// the caller because every send path has already loaded the user — re-reading it here would
// be a query per message for a value we are holding.
func (s *Service) recordOutbound(integration *models.Integration, msgType, body string, providerMsgID *string, status string, retentionDays int, spend Spend) {
	if providerMsgID != nil && *providerMsgID == "" {
		providerMsgID = nil
	}
	out := models.ChannelMessage{
		BindingID:         integration.ID,
		UserID:            integration.UserID,
		Provider:          integration.Provider,
		Direction:         models.ChannelMessageOutbound,
		ProviderMessageID: providerMsgID,
		Type:              msgType,
		Body:              &body,
		Status:            status,
		ExpiresAt:         models.ChatMessageExpiry(time.Now(), retentionDays),
	}
	var msgID *string
	if err := database.DB.Create(&out).Error; err != nil {
		s.logger.Warn("companion: failed to record outbound message", zap.Error(err))
	} else {
		msgID = &out.ID
	}
	// The usage row is written even when the erasable message row above failed —
	// it is the record billing counts on, not an annotation of the message.
	s.recordUsage(integration, msgID, spend)

	// Stamp the binding too — this, not the row above, is what the proactive quiet period
	// reads, so it must survive the row being purged. Failed sends stamp it as well: that
	// is what counting the log used to do (the count had no status filter), and staying
	// quiet after a failure is the safer of the two readings anyway.
	now := time.Now()
	integration.LastOutboundAt = &now
	if err := database.DB.Model(integration).Update("last_outbound_at", now).Error; err != nil {
		s.logger.Warn("companion: failed to update last_outbound_at", zap.Error(err))
	}
}

// recordUsage lands one turn's spend in the cost ledger (ai_usage_records), whatever
// way the turn ended — a failed call burned its tokens all the same. It used to also
// write a channel_usage_records event row; that table lost its last reader when
// /me/usage narrowed to charged replies, and is now legacy-orphaned like goals.
//
// aiusage.Write is log-and-continue and drops empty spends itself: metering must
// never break message handling, mirroring how session-end billing treats debits.
func (s *Service) recordUsage(integration *models.Integration, messageID *string, spend Spend) {
	// Each model priced on its own terms. Transcription and synthesis run on
	// different models from the chat turn, at prices that differ by more than a
	// rounding error — TTS output is $20.00 per million tokens against $9.00 for
	// chat — so a single attribution would be wrong in both directions at once.
	aiusage.Write(context.Background(), aiusage.Record{
		UserID:       integration.UserID,
		Kind:         models.AIUsageMessage,
		Model:        chatModel(),
		RefType:      models.AIUsageRefMessage,
		RefID:        deref(messageID),
		InputTokens:  spend.Chat.InputTokens,
		OutputTokens: spend.Chat.OutputTokens,
		TotalTokens:  spend.Chat.TotalTokens,
		STT:          auxSpend(transcribeModel(), spend.STT),
		TTS:          auxSpend(ttsModel(), spend.TTS),
	})
}

// auxSpend labels an auxiliary model's tokens with the model that earned them, and
// returns the zero value when there are none — an empty model is how the ledger knows
// the group is unused.
func auxSpend(model string, u Usage) aiusage.AuxSpend {
	if (u == Usage{}) {
		return aiusage.AuxSpend{}
	}
	return aiusage.AuxSpend{
		Model:        model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
	}
}

func (s *Service) markMessage(msg *models.ChannelMessage, status string) {
	msg.Status = status
	if err := database.DB.Model(msg).Update("status", status).Error; err != nil {
		s.logger.Warn("companion: failed to update message status", zap.Error(err))
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
