package companion

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	dispatcherInterval = 60 * time.Second
	drainBatchSize     = 10
	// nudgeEnqueueLockKey serializes daily-nudge enqueueing across replicas
	// (pg advisory xact lock).
	nudgeEnqueueLockKey = 771001
)

// StartDispatcher runs the proactive-messaging loop: draining due
// channel_follow_ups rows and enqueueing daily nudges. Safe with multiple
// replicas (FOR UPDATE SKIP LOCKED + advisory lock).
func StartDispatcher(svc *Service, logger *zap.Logger) {
	go func() {
		ticker := time.NewTicker(dispatcherInterval)
		defer ticker.Stop()
		ticks := 0
		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("companion dispatcher: recovered from panic", zap.Any("panic", r))
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), dispatcherInterval)
				defer cancel()
				svc.drainFollowUps(ctx)
				svc.enqueueDailyNudges(time.Now().UTC())
				// Retention is measured in days; sweeping every minute would query the
				// table sixty times an hour to delete nothing.
				if ticks%int(time.Hour/dispatcherInterval) == 0 {
					svc.purgeExpiredMessages()
				}
				ticks++
			}()
		}
	}()
}

// drainFollowUps claims and sends due follow-ups. Rows are locked with SKIP
// LOCKED and marked sent/failed within the same transaction, so concurrent
// replicas never double-send; sends happen inside the transaction, which is
// acceptable at this batch size (≤10 per minute).
func (s *Service) drainFollowUps(ctx context.Context) {
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var due []models.ChannelFollowUp
		if err := tx.Raw(`
			SELECT * FROM channel_follow_ups
			WHERE sent_at IS NULL AND failed_at IS NULL AND scheduled_at <= NOW()
			ORDER BY scheduled_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, drainBatchSize).Scan(&due).Error; err != nil {
			return err
		}
		now := time.Now()
		for i := range due {
			followUp := &due[i]
			column := "sent_at"
			if err := s.sendFollowUp(ctx, followUp); err != nil {
				s.logger.Warn("companion dispatcher: follow-up send failed",
					zap.Error(err), zap.String("followUpID", followUp.ID), zap.String("kind", followUp.Kind))
				column = "failed_at"
			}
			updates := map[string]any{column: now}
			// sendFollowUp stamps the delivered text on the row; persisting it is
			// what future proactive generations read to avoid repeating it.
			if followUp.SentText != nil {
				updates["sent_text"] = *followUp.SentText
			}
			if err := tx.Model(followUp).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.logger.Error("companion dispatcher: drain failed", zap.Error(err))
	}
}

// sendFollowUp delivers one proactive message, honoring WhatsApp's 24h
// customer-service window: free-form inside it, approved template outside.
// proactiveQuietPeriod is how long a proactive message reserves the conversation. Two
// unprompted messages close together read as automation no matter how well written.
const proactiveQuietPeriod = 6 * time.Hour

// mayReachOutNow reports whether we may send an UNPROMPTED message on this channel right
// now. Every proactive path goes through here — the post-session follow-up, the daily
// nudge, and scheduled notifications composed into chat — because they run in separate
// loops and would otherwise each be individually well-behaved while landing on top of
// one another. Replies to a user's own message are not proactive and never gated: if
// someone is talking to us, we answer.
// The gate reads Integration.LastOutboundAt rather than counting channel_messages: the
// message log is erasable (DELETE /me/data?scope=chat), and a guard derived from it
// vanished with it — a user who cleared their chat could be messaged again on the spot.
func mayReachOutNow(integration *models.Integration) bool {
	return integration.MayReachOutAfter(proactiveQuietPeriod)
}

func (s *Service) sendFollowUp(ctx context.Context, followUp *models.ChannelFollowUp) error {
	var integration models.Integration
	if err := database.DB.First(&integration, "id = ? AND status = ?", followUp.BindingID, models.IntegrationActive).Error; err != nil {
		return fmt.Errorf("integration gone or inactive: %w", err)
	}
	var user models.User
	if err := database.DB.First(&user, "id = ?", followUp.UserID).Error; err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	channel, err := messaging.Get(integration.Provider)
	if err != nil {
		return err
	}

	if !mayReachOutNow(&integration) {
		return fmt.Errorf("a message was already sent recently; leaving room")
	}

	if integration.CanSendFreeForm(serviceWindow) {
		directive := proactiveBaseDirective + s.recentProactiveBlock(integration.ID)
		switch followUp.Kind {
		case models.ChannelFollowUpDailyNudge:
			if followUp.PayloadHint != nil && *followUp.PayloadHint != "" {
				directive += fmt.Sprintf("\n\nTHIS MESSAGE: their journey has a '%s' session waiting for them in the app. Open with the real thing it would help them with — drawn from their context, in their words where you can — and let the invitation follow from that in a short second sentence. Never lead with the invitation, never describe the session as a product or a feature, and if they did a session very recently, do not push another one: just be with them instead.", *followUp.PayloadHint)
			} else {
				directive += "\n\nTHIS MESSAGE: nothing specific is scheduled. Pick the ONE thing from their context most alive right now — a commitment they are in the middle of, something they realised in their last session, a habit ending soon — and speak to that. If there is genuinely nothing to point at, say something short and human and leave the door open; do not manufacture a reason."
			}
		case models.ChannelFollowUpPostSession:
			directive += "\n\nTHIS MESSAGE: they finished a session a few hours ago. Reach for the specific thing that came up in it — their own words if you have them — and ask something small about how it has sat with them since, or how the first step is going. Do not summarise the session back to them, and do not congratulate them for attending."
		}
		text, sent := s.generateAndSendSystemReply(ctx, channel, &integration, &user, directive,
			s.proactiveFallback(&user))
		if !sent {
			return fmt.Errorf("proactive send failed")
		}
		// Stamped in memory; the drain loop persists it alongside sent_at. This is
		// the durable trace future generations read to avoid repeating themselves.
		followUp.SentText = &text
		return nil
	}

	// Outside the window: only a pre-approved template may be sent.
	tmpl := messaging.TemplateMessage{
		Name:     config.AppConfig.WhatsAppTemplateReengage,
		Language: metaLocale(user.PreferredLanguage),
	}
	providerMsgID, err := channel.SendTemplate(ctx, messaging.Address{Provider: integration.Provider, ExternalID: deref(integration.ExternalID)}, tmpl)
	if err != nil {
		return fmt.Errorf("send template: %w", err)
	}
	// Templates are pre-approved static text — no model call, zero tokens, but
	// still a sent message the usage ledger must count.
	s.recordOutbound(&integration, models.ChannelMessageTypeTemplate, tmpl.Name, &providerMsgID, models.ChannelMessageSent, user.ChatHistoryRetentionDays, Spend{})
	return nil
}

// enqueueDailyNudges creates one daily_nudge follow-up per active integration at
// the configured UTC hour, skipping users who already chatted today. An
// advisory lock + per-day existence check keep it idempotent across replicas.
func (s *Service) enqueueDailyNudges(now time.Time) {
	if now.Hour() != config.AppConfig.CompanionNudgeHourUTC {
		return
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var locked bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", nudgeEnqueueLockKey).Scan(&locked).Error; err != nil {
			return err
		}
		if !locked {
			return nil // another replica is enqueueing this minute
		}

		var integrations []models.Integration
		if err := tx.Where("status = ?", models.IntegrationActive).Find(&integrations).Error; err != nil {
			return err
		}
		for _, integration := range integrations {
			// Skip users who were already active on the channel today.
			if integration.LastInboundAt != nil && integration.LastInboundAt.After(dayStart) {
				continue
			}
			// Same-day dedupe. This one may legitimately vanish (scope=chat drops queued
			// follow-ups), but a re-enqueued nudge cannot turn into a duplicate message:
			// the send still passes through mayReachOutNow, which now reads the binding.
			var existing int64
			if err := tx.Model(&models.ChannelFollowUp{}).
				Where("binding_id = ? AND kind = ? AND created_at >= ?", integration.ID, models.ChannelFollowUpDailyNudge, dayStart).
				Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}

			var hint *string
			if planned := journey.PlannedSessionForToday(integration.UserID, time.UTC); planned != "" {
				h := string(planned)
				hint = &h
			}
			followUp := models.ChannelFollowUp{
				UserID:      integration.UserID,
				BindingID:   integration.ID,
				Kind:        models.ChannelFollowUpDailyNudge,
				PayloadHint: hint,
				ScheduledAt: now,
			}
			if err := tx.Create(&followUp).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.logger.Error("companion dispatcher: nudge enqueue failed", zap.Error(err))
	}
}

// purgeExpiredMessages deletes chat messages whose retention has run out, and drained
// follow-up rows past their own horizon.
//
// It replaces an inactivity-based purge that had the wrong shape for a retention policy:
// it removed a whole conversation once it went quiet, which meant somebody who wrote every
// day never had anything deleted, however many months piled up — precisely the case where
// retention matters. Expiry is per message, materialized on write from the user's setting,
// so this needs no join and no knowledge of users at all.
//
// Deleting in batches keeps any single statement short: the first sweep after a retention
// is lowered can match a great many rows, and a long DELETE holding locks is how a
// background job turns into an incident. Idempotent, so replicas are harmless.
func (s *Service) purgeExpiredMessages() {
	const batch = 5000

	total := int64(0)
	for {
		res := database.DB.Exec(`
			DELETE FROM channel_messages WHERE id IN (
				SELECT id FROM channel_messages
				WHERE expires_at IS NOT NULL AND expires_at < ?
				LIMIT ?
			)`, time.Now(), batch)
		if res.Error != nil {
			s.logger.Error("companion dispatcher: message retention sweep failed", zap.Error(res.Error))
			return
		}
		total += res.RowsAffected
		if res.RowsAffected < batch {
			break
		}
	}

	// Follow-ups: only terminal rows go — anything still pending is work not yet done.
	// Drained rows carry sent_text (the delivered proactive copy, kept as anti-repetition
	// memory), so this sweep is also the privacy bound on that text.
	cutoff := time.Now().AddDate(0, 0, -followUpRetentionDays)
	res := database.DB.Exec(
		`DELETE FROM channel_follow_ups WHERE (sent_at IS NOT NULL OR failed_at IS NOT NULL) AND created_at < ?`,
		cutoff)
	if res.Error != nil {
		s.logger.Error("companion dispatcher: follow-up sweep failed", zap.Error(res.Error))
	}

	if total > 0 || res.RowsAffected > 0 {
		s.logger.Info("companion dispatcher: retention sweep",
			zap.Int64("messagesDeleted", total),
			zap.Int64("followUpsDeleted", res.RowsAffected))
	}
}

// metaLocale maps a BCP-47 preferred language (pt-PT) to Meta's template
// locale format (pt_PT), defaulting to en.
func metaLocale(preferred *string) string {
	if preferred == nil || *preferred == "" {
		return "en"
	}
	return strings.ReplaceAll(*preferred, "-", "_")
}

// ComposeScheduledNotification turns a scheduled notification into a real message in the
// user's chat. The notification's title and body were written for a push — an imperative
// nudge at an app icon — so they are used as the INTENT of what to say, never as the text
// to send. The companion rewrites it as one short message in the user's language, with
// their full context (commitments, sessions, journey), so it reads as the same person
// they were just talking to rather than an automated notice.
//
// Wired into notification.ChatComposer at startup.
func (s *Service) ComposeScheduledNotification(ctx context.Context, userID, integrationID, title, message string) error {
	var integration models.Integration
	if err := database.DB.First(&integration, "id = ? AND status = ?", integrationID, models.IntegrationActive).Error; err != nil {
		return fmt.Errorf("integration gone or inactive: %w", err)
	}
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	channel, err := messaging.Get(integration.Provider)
	if err != nil {
		return err
	}
	if !integration.CanSendFreeForm(serviceWindow) {
		// Outside the free-form window only a pre-approved template may be sent, and a
		// template cannot carry this. Refusing lets the dispatcher fall through to push,
		// which is the right channel when we cannot actually hold a conversation.
		return fmt.Errorf("outside the service window")
	}

	if !mayReachOutNow(&integration) {
		return fmt.Errorf("a message was already sent recently; leaving room")
	}

	directive := proactiveBaseDirective + s.recentProactiveBlock(integration.ID) + fmt.Sprintf(
		"\n\nTHIS MESSAGE: your own note-to-self for this moment reads %q — %q. That note was written as a reminder, not as something to say: it tells you WHAT you wanted to raise, never HOW to say it. Say it as yourself, to this person, now. Do not read the note out, do not keep its title, and do not let it turn the message into an announcement. If it points them back to the app, that is an invitation from you, grounded in why it would help THEM.",
		title, message)

	text, sent := s.generateAndSendSystemReply(ctx, channel, &integration, &user, directive, s.proactiveFallback(&user))
	if !sent {
		// Failing loudly lets the notification dispatcher fall through to push —
		// reporting success on a dead send would swallow the notification entirely.
		return fmt.Errorf("chat delivery failed")
	}

	// Log-only follow-up row (already drained: SentAt set on insert, so the drain
	// loop never picks it up). Composed notifications are unprompted messages like
	// any other and must be visible to the anti-repetition block above.
	now := time.Now()
	logRow := models.ChannelFollowUp{
		UserID:      userID,
		BindingID:   integration.ID,
		Kind:        models.ChannelFollowUpScheduledNotification,
		ScheduledAt: now,
		SentAt:      &now,
		SentText:    &text,
	}
	if err := database.DB.Create(&logRow).Error; err != nil {
		s.logger.Warn("companion: failed to log composed notification text", zap.Error(err))
	}
	return nil
}

// recentProactiveTexts / recentProactiveBlock bound how much of the proactive send
// history is replayed into a generation.
const (
	recentProactiveLimit  = 5
	recentProactiveWindow = 14 * 24 * time.Hour
)

// recentProactiveBlock renders the last few unprompted messages sent on this binding as a
// directive section, so the model can avoid rerunning them. It reads channel_follow_ups
// (sent_text), NOT the conversation history: the ephemeral purge erases channel_messages
// after a few quiet hours, and quiet users are exactly who proactive messages go to — by
// the time today's nudge is generated, yesterday's is usually gone from the log. Returns
// "" when there is nothing to show; errors degrade to that, never block the send.
func (s *Service) recentProactiveBlock(bindingID string) string {
	var texts []string
	since := time.Now().Add(-recentProactiveWindow)
	if err := database.DB.Model(&models.ChannelFollowUp{}).
		Where("binding_id = ? AND sent_at IS NOT NULL AND sent_text IS NOT NULL AND sent_at >= ?", bindingID, since).
		Order("sent_at desc").
		Limit(recentProactiveLimit).
		Pluck("sent_text", &texts).Error; err != nil {
		s.logger.Warn("companion: failed to load recent proactive messages", zap.Error(err))
		return ""
	}
	if len(texts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\nYOUR RECENT UNPROMPTED MESSAGES, newest first (the conversation above may no longer contain them):\n")
	for i, t := range texts {
		fmt.Fprintf(&b, "%d. %q\n", i+1, t)
	}
	b.WriteString("Today's message must not read as a rerun of any of these: different topic, different opening, different question. If the one thing you would naturally raise is already on this list and nothing about it has changed, pick a different thread from their context instead — and if there is truly nothing new, keep it to one short, simple human line rather than repeating yourself.")
	return b.String()
}

// proactiveBaseDirective governs every unprompted message Rumi sends. The old version
// said little more than "check in on them", which invites exactly the message that makes
// people mute a bot: a greeting, a generic "how are you?", nothing they can answer. The
// system prompt already carries their commitments, sessions, insights and streak — the
// job of this directive is to force the message to USE that, and to sound like a person
// who remembered something rather than a service running a job.
//
// Each caller appends a "THIS MESSAGE:" section naming the one thing this particular
// message is about.
const proactiveBaseDirective = `SYSTEM: You are writing to the user unprompted — they did not just message you, and they are not waiting for a reply. This is the standard a message has to meet to be worth interrupting someone's day.

WHAT TO SAY
- Be about ONE specific thing, taken from what you actually know about them (their commitments, what they realised in a session, a habit ending soon, how long it has been). A message that could have been sent to any user is a failure, however warm it sounds.
- Never open with "how are you?", "how's it going?", "just checking in", or any variation. Those are what you write when you have nothing to say — and you do have something.
- Do not summarise their progress back at them, do not list their commitments, and do not report statistics. Mention at most one concrete thing.
- If something has slipped, approach it with curiosity, never with guilt, disappointment, or a nudge to catch up. Missing days is information about the plan, never a failure of theirs.

HOW TO SAY IT
- ONE message, one or two sentences. This is a text, not an email or a coaching intervention.
- End with at most ONE light question they can answer in a few words while walking. No stacked questions, no homework, and it is fine to end with no question at all when the moment calls for a simple statement.
- Write in their language, in your own voice: plain, warm, specific. No markdown, no bullet points, no headings, no emoji beyond a single one where it genuinely fits.
- Do not greet them by name if you have spoken recently; walking up mid-relationship is warmer than starting over each time.
- Do NOT reuse the opening, structure, topic, or question of your previous unprompted messages — check the conversation above AND the list of your recent unprompted messages when one follows. Repetition across days is what makes this feel automated more than anything else.
- Never mention notifications, reminders, schedules, systems, or that you were prompted to write. As far as the user is concerned, you thought of them.`

// proactiveFallback is the message sent when generation fails. It must be in the user's
// own language: a hardcoded English line reaching a Portuguese user is worse than the
// silence it replaces, because it exposes the machinery.
func (s *Service) proactiveFallback(user *models.User) string {
	lang := "en-US"
	if user != nil && user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
		lang = *user.PreferredLanguage
	}
	if s.localizer != nil {
		if text := s.localizer.Get(lang, "companion.proactive_fallback", nil); text != "" {
			return text
		}
	}
	return "Hey — thinking of you. How are things going?"
}
