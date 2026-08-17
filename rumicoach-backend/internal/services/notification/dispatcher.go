package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	dispatcherInterval = 60 * time.Second
	drainBatchSize     = 20
	// whatsAppServiceWindow is WhatsApp's 24h customer-service window: free-form
	// messages may only be sent within it (mirrors the companion dispatcher).
	whatsAppServiceWindow = 24 * time.Hour
)

// StartDispatcher runs the notification delivery loop: draining due
// notifications rows and delivering each over exactly one channel, chosen at
// send time. Safe with multiple replicas (FOR UPDATE SKIP LOCKED).
func StartDispatcher(logger *zap.Logger) {
	go func() {
		ticker := time.NewTicker(dispatcherInterval)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("notification dispatcher: recovered from panic", zap.Any("panic", r))
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), dispatcherInterval)
				defer cancel()
				drainNotifications(ctx, logger)
			}()
		}
	}()
}

// minGapBetweenNotifications is the minimum spacing between two notifications for the
// same user, whatever the channel. A coaching session ends by scheduling several at
// once; delivered as a batch they read as a system flushing a queue rather than someone
// thinking of you. Deferring costs nothing — the notification still arrives, later.
const minGapBetweenNotifications = 6 * time.Hour

// timeSensitiveGrace is how late a moment-bound message may still be delivered. Past
// this it is dropped: a message tied to something in the user's day has no value once
// that part of the day is over.
const timeSensitiveGrace = 2 * time.Hour

// deferUntil decides whether a notification must wait so it does not land on top of the
// previous one, and when it should be retried. lastSent is the user's most recent
// delivery, nil when they have never had one.
func deferUntil(lastSent *time.Time, now time.Time) (time.Time, bool) {
	if lastSent == nil || lastSent.IsZero() {
		return time.Time{}, false
	}
	elapsed := now.Sub(*lastSent)
	// A delivery timestamp in the future means clock skew, not a real send; treating it
	// as recent would stall the user's notifications until the clocks agree.
	if elapsed < 0 || elapsed >= minGapBetweenNotifications {
		return time.Time{}, false
	}
	return lastSent.Add(minGapBetweenNotifications), true
}

// drainNotifications claims and delivers due notifications. Rows are locked
// with SKIP LOCKED and marked sent/failed within the same transaction, so
// concurrent replicas never double-send (same pattern as the companion
// follow-up dispatcher).
func drainNotifications(ctx context.Context, logger *zap.Logger) {
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var due []models.Notification
		if err := tx.Raw(`
			SELECT * FROM notifications
			WHERE sent_at IS NULL AND failed_at IS NULL AND scheduled_at <= NOW()
			ORDER BY scheduled_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, drainBatchSize).Scan(&due).Error; err != nil {
			return err
		}
		now := time.Now()
		for i := range due {
			n := &due[i]

			// Space them out per user. A session end schedules SEVERAL notifications at
			// once, so without this a batch arrives back-to-back — a burst of pushes, or
			// worse, several messages in a row in a conversation. Anything still inside
			// the quiet window is pushed forward rather than dropped: the user gets it,
			// just not stacked on the last one. Because sent_at is written inside this
			// same transaction, two notifications for one user in the SAME batch also
			// space correctly — the second sees the first as already sent.
			// A time-sensitive message is bound to a moment in the user's life, so it is
			// never shifted to make room for another: deferring "good luck this morning"
			// by six hours delivers it after the exam. It goes now, and the spacing rule
			// applies to everything around it instead.
			var lastSent *time.Time
			if n.TimeSensitive {
				logger.Info("notification dispatcher: delivering time-sensitive message without spacing",
					zap.String("notificationID", n.ID), zap.String("userID", n.UserID))
			} else if err := tx.Raw(`
				SELECT MAX(sent_at) FROM notifications
				WHERE user_id = ? AND sent_at IS NOT NULL`, n.UserID).Scan(&lastSent).Error; err != nil {
				logger.Warn("notification dispatcher: could not check spacing, delivering anyway",
					zap.Error(err), zap.String("userID", n.UserID))
			} else if next, defer_ := deferUntil(lastSent, now); defer_ {
				if err := tx.Model(n).Update("scheduled_at", next).Error; err != nil {
					return err
				}
				logger.Info("notification dispatcher: deferred to keep messages apart",
					zap.String("notificationID", n.ID), zap.String("userID", n.UserID),
					zap.Time("rescheduledFor", next))
				continue
			}

			// If a time-sensitive message missed its moment (the dispatcher was down, or
			// it queued behind others), sending it late is worse than not sending it.
			if n.TimeSensitive && now.Sub(n.ScheduledAt) > timeSensitiveGrace {
				if err := tx.Model(n).Update("failed_at", now).Error; err != nil {
					return err
				}
				logger.Info("notification dispatcher: dropped a time-sensitive message that missed its moment",
					zap.String("notificationID", n.ID), zap.String("userID", n.UserID),
					zap.Time("scheduledFor", n.ScheduledAt))
				continue
			}

			via, err := Deliver(ctx, logger, n)
			updates := map[string]any{}
			if err != nil {
				logger.Warn("notification dispatcher: delivery failed",
					zap.Error(err), zap.String("notificationID", n.ID), zap.String("userID", n.UserID))
				updates["failed_at"] = now
			} else {
				logger.Info("notification dispatcher: delivered",
					zap.String("notificationID", n.ID), zap.String("userID", n.UserID), zap.String("via", via))
				updates["sent_at"] = now
				updates["sent_via"] = via
			}
			if err := tx.Model(n).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Error("notification dispatcher: drain failed", zap.Error(err))
	}
}

// ChatComposer turns a scheduled notification into a conversational message on the
// user's chat channel, written by the companion with their full context. It is injected
// at startup (main.go) rather than imported, so this package stays free of any
// dependency on the companion service. Nil in deployments without a messaging channel,
// in which case the scheduled text is sent as-is.
//
// It must return an error — not silently do nothing — when composition fails, so the
// dispatcher can fall back to sending the scheduled text rather than dropping the
// notification entirely.
var ChatComposer func(ctx context.Context, userID, integrationID, title, message string) error

// Deliver sends one notification over exactly one channel, chosen now (never
// at scheduling time): the user's active messaging channels are tried first,
// in integration-creation order, and FCM push is the last resort, used only when
// no messaging channel can deliver. A user with a working chat channel therefore
// never gets a push for the same notification — the two would be the same nudge
// twice, in two registers. Returns the channel that delivered.
func Deliver(ctx context.Context, logger *zap.Logger, n *models.Notification) (string, error) {
	var integrations []models.Integration
	if err := database.DB.
		Where("user_id = ? AND status = ?", n.UserID, models.IntegrationActive).
		Order("created_at").
		Find(&integrations).Error; err != nil {
		logger.Warn("notification dispatcher: failed to load channel integrations, falling back to push",
			zap.Error(err), zap.String("userID", n.UserID))
		integrations = nil
	}

	text := n.Title + "\n\n" + n.Message
	for i := range integrations {
		integration := &integrations[i]
		if !canSendFreeForm(integration) {
			continue
		}
		channel, err := messaging.Get(integration.Provider)
		if err != nil {
			continue // provider not wired in this deployment
		}

		// A notification and a chat message are different products, not two renderings
		// of the same one. Push copy is an imperative nudge at an app icon, read in two
		// seconds; the same words in a conversation — from someone the user was talking
		// to hours ago — read as marketing and undermine the relationship the companion
		// is building. So on a chat channel we send the notification's INTENT to the
		// companion and let it write a real message, in the user's language and with
		// their context. The scheduled text is the fallback if that fails.
		if ChatComposer != nil {
			if err := ChatComposer(ctx, n.UserID, integration.ID, n.Title, n.Message); err == nil {
				logger.Info("notification delivered as a companion message",
					zap.String("provider", integration.Provider), zap.String("notificationID", n.ID))
				return integration.Provider, nil
			} else {
				logger.Warn("notification dispatcher: companion composition failed, sending the scheduled text",
					zap.Error(err), zap.String("notificationID", n.ID))
			}
		}

		providerMsgID, err := channel.SendText(ctx, messaging.Address{Provider: integration.Provider, ExternalID: derefString(integration.ExternalID)}, text)
		if err != nil {
			logger.Warn("notification dispatcher: channel send failed, trying next channel",
				zap.Error(err), zap.String("provider", integration.Provider), zap.String("notificationID", n.ID))
			continue
		}
		recordChannelMessage(logger, integration, text, providerMsgID)
		return integration.Provider, nil
	}

	if GlobalNotificationService == nil {
		return "", fmt.Errorf("no messaging channel available and notification service not initialized")
	}
	if err := GlobalNotificationService.SendPushNotification(ctx, n.UserID, n.Title, n.Message); err != nil {
		return "", fmt.Errorf("push fallback failed: %w", err)
	}
	return models.NotificationChannelPush, nil
}

// canSendFreeForm reports whether a integration may receive a free-form message
// right now. WhatsApp only allows free-form sends inside the 24h
// customer-service window; the personalized notification text cannot go into a
// pre-approved template, so outside the window the channel is not available
// and delivery falls through to the next channel (ultimately push).
func canSendFreeForm(integration *models.Integration) bool {
	if integration.Provider == models.ChannelProviderWhatsApp {
		return integration.InsideServiceWindow(whatsAppServiceWindow)
	}
	return true
}

// recordChannelMessage persists the outbound send on the channel's message log
// so the companion model sees it as part of the conversation history.
func recordChannelMessage(logger *zap.Logger, integration *models.Integration, body, providerMsgID string) {
	var msgID *string
	if providerMsgID != "" {
		msgID = &providerMsgID
	}
	out := models.ChannelMessage{
		BindingID:         integration.ID,
		UserID:            integration.UserID,
		Provider:          integration.Provider,
		Direction:         models.ChannelMessageOutbound,
		ProviderMessageID: msgID,
		Type:              models.ChannelMessageTypeText,
		Body:              &body,
		Status:            models.ChannelMessageSent,
	}
	if err := database.DB.Create(&out).Error; err != nil {
		logger.Warn("notification dispatcher: failed to record outbound channel message", zap.Error(err))
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
