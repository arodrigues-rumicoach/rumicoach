package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/gorm"
)

// dataScope is one category of user data that DELETE /me/data can erase on its own, so
// somebody can clear their chat without losing their journey.
type dataScope string

const (
	scopeMemories    dataScope = "memories"
	scopeChat        dataScope = "chat"
	scopeCommitments dataScope = "commitments"
	scopeProgress    dataScope = "progress"
	scopeAll         dataScope = "all"
)

// validScopes is the whole vocabulary, in the order the error message lists it.
var validScopes = []dataScope{scopeMemories, scopeChat, scopeCommitments, scopeProgress, scopeAll}

// parseDataScopes turns the ?scope= value into the scopes to run, de-duplicated and in
// the order given. An empty value means "all", so callers written before the parameter
// existed keep working unchanged. "all" absorbs anything it is combined with — but only
// after every value has been validated, so a typo is still rejected rather than silently
// swallowed by the broadest scope.
func parseDataScopes(raw string) ([]dataScope, error) {
	if strings.TrimSpace(raw) == "" {
		return []dataScope{scopeAll}, nil
	}

	var out []dataScope
	seen := make(map[dataScope]bool, len(validScopes))
	hasAll := false

	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		s := dataScope(strings.ToLower(trimmed))
		if !isValidScope(s) {
			names := make([]string, len(validScopes))
			for i, v := range validScopes {
				names[i] = string(v)
			}
			return nil, fmt.Errorf("unknown scope %q; valid values are: %s", trimmed, strings.Join(names, ", "))
		}
		if s == scopeAll {
			hasAll = true
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}

	if hasAll {
		return []dataScope{scopeAll}, nil
	}
	if len(out) == 0 {
		return []dataScope{scopeAll}, nil
	}
	return out, nil
}

func isValidScope(s dataScope) bool {
	for _, v := range validScopes {
		if v == s {
			return true
		}
	}
	return false
}

// dropBadges removes specific awarded badges. Badges are otherwise never revoked
// (badge.EvaluateAndAward only inserts), but every criterion re-reads live tables — so
// erasing a category without dropping the badges it earned leaves the grid asserting an
// achievement next to a counter reading zero, on the same screen.
func dropBadges(tx *gorm.DB, userID string, badges ...api.BadgeType) error {
	if len(badges) == 0 {
		return nil
	}
	types := make([]string, len(badges))
	for i, b := range badges {
		types[i] = string(b)
	}
	return tx.Where("user_id = ? AND badge_type IN ?", userID, types).
		Delete(&models.UserBadge{}).Error
}

// deleteMemoriesScope erases the saved memories. The cleanest of the scopes: nothing
// breaks, Rumi simply forgets. The knock-on effects — insightsDiscovered falling to
// zero, the coach's prompt losing its memory block, future summaries having no
// key_insight — ARE the user's intent and are deliberately not compensated for.
// identity_reflections go with the memories: the reflection is the same kind of
// personal record (who the user learned to be, what it costs them), injected into the
// coach's prompt the same way the memory block is.
func deleteMemoriesScope(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.UserMemory{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.IdentityReflection{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.AcceptanceReflection{}).Error; err != nil {
		return err
	}
	// tenInsights counts user_memories where category = 'insight'.
	return dropBadges(tx, userID, api.TenInsights)
}

// deleteChatScope clears the WhatsApp/Telegram conversation. It deliberately does NOT
// touch the integrations row: this clears the conversation, it does not disconnect the
// channel. Queued-but-unsent proactive messages go with it, which is the point — and so
// do the drained follow-up rows, whose sent_text holds delivered proactive copy (the
// anti-repetition memory), which is conversation content and must not outlive an erasure.
//
// Safe only because the two chat rate limits no longer derive from this table — they
// live on the binding (Integration.LastOutboundAt, DailyInboundCount). Reading them from
// the message log meant a user who cleared their chat for privacy could be messaged
// again immediately, and could clear it deliberately to reset their daily cap.
//
// The accounting records survive every erasure of the conversation they meter:
// message charges live in balance_transactions and token spend in ai_usage_records,
// and neither is touched here — same rule as everywhere else.
func deleteChatScope(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.ChannelMessage{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ?", userID).Delete(&models.ChannelFollowUp{}).Error
}

// deleteCommitmentsScope erases commitments and everything that executes them.
//
// behavior_plans go too because BehaviorPlan.TaskID points at the recurring Commitment
// that carries daily execution: delete the commitment and keep the plan, and the next
// sync updates zero rows in silence while the habit never re-projects. behavior_check_ins
// go because commitmentsKept is `commitments WHERE done` PLUS `behavior_check_ins WHERE
// kept` — dropping the two badges while half their input survives just re-awards them on
// the next evaluation.
func deleteCommitmentsScope(tx *gorm.DB, userID string) error {
	// Completions first: they key off commitments and would otherwise be orphaned rows
	// that the companion's accountability context still joins in memory.
	if err := tx.Where("user_id = ?", userID).Delete(&models.CommitmentCompletion{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.Commitment{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorPlan{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorCheckIn{}).Error; err != nil {
		return err
	}
	return dropBadges(tx, userID, api.FirstCommitment, api.TwentyFiveCommitments)
}

// redactSessionContent strips what a session row holds in the user's own voice — the
// transcript, the recap written back to them, their rating, their feedback, the insight
// they named, the summary payload — while keeping the row.
//
// What stays is everything the business needs the row for: the measurements (timings,
// duration, session type, per-modality token counts, Deepgram seconds, STT service) and
// the QA review of the COACH (ai_notes, ai_evaluation), which grades Rumi's own conduct
// rather than recording the user. This is the same split we drew between
// balance_transactions and the coaching tables, applied inside a single row.
//
// Caveat worth knowing: ai_notes is generated FROM the transcript, so it can paraphrase
// what the user said while explaining what the coach got wrong. It is internal, English,
// and never shown to the user — but it is not guaranteed free of their content.
func redactSessionContent(tx *gorm.DB, userID string) error {
	return tx.Model(&models.CommunicationSession{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"transcript":           nil,
			"recap":                nil,
			"recap_title":          nil,
			"user_evaluation":      nil,
			"user_feedback":        nil,
			"user_session_insight": nil,
			"session_summary":      nil,
		}).Error
}

// deleteTodaysGrowth removes only the CURRENT day's snapshot. Nothing reads the historical
// rows — /journey and PlannedSessionForToday both look up the row for today's date
// and nothing else — and what they record (which session was proposed on which day, which
// quote went with it) is product analytics with no personal content in it. Today's row
// must go, though, or the endpoint reuses the stale proposal and the reset looks like it
// never happened.
func deleteTodaysGrowth(tx *gorm.DB, userID string, loc *time.Location) error {
	if loc == nil {
		loc = time.UTC
	}
	today := time.Now().In(loc).Format("2006-01-02")
	return tx.Where("user_id = ? AND date = ?", userID, today).Delete(&models.DailyJourney{}).Error
}

// redactNotifications keeps the delivery telemetry of notifications that already went out
// — when they were scheduled, when they fired, and over which channel (sent_via is how we
// know whether chat or push actually reached the user) — and strips the copy, which was
// written about them. Anything still pending is deleted outright: a queued nudge must not
// fire after the user erased the conversation that produced it, least of all with its text
// blanked out.
func redactNotifications(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ? AND sent_at IS NULL", userID).
		Delete(&models.Notification{}).Error; err != nil {
		return err
	}
	// Title and Message are NOT NULL, so they are emptied rather than nulled.
	return tx.Model(&models.Notification{}).
		Where("user_id = ? AND sent_at IS NOT NULL", userID).
		Updates(map[string]interface{}{"title": "", "message": ""}).Error
}

// markJourneyReset draws the line the journey starts counting from. Session rows survive
// every scope — they carry the duration and token analytics — so this stamp is the only
// thing that makes "start over" real: everything reading progress out of that table goes
// through models.JourneySessions, which filters on it.
func markJourneyReset(tx *gorm.DB, userID string) error {
	return tx.Model(&models.User{}).Where("id = ?", userID).
		Update("journey_reset_at", time.Now()).Error
}

// deleteProgressScope erases what the user reads as "my progress": today's growth
// snapshot, the content of past sessions, the badges earned from session and streak
// counters, and the line the journey counts from.
//
// user_app_opens is deliberately NOT touched — it holds a user id and a date, nothing
// else, is read by no code path at all since the streak moved to session history, and is
// the retention record for the account. Deleting it was pure loss.
//
// Neither are the session rows themselves. The user is still allowed to SEE that those
// sessions happened — the history list and the usage calendar are unfiltered on purpose.
// What resets is what counts, not what is on the record.
func deleteProgressScope(tx *gorm.DB, userID string, loc *time.Location) error {
	if err := deleteTodaysGrowth(tx, userID, loc); err != nil {
		return err
	}
	if err := redactSessionContent(tx, userID); err != nil {
		return err
	}
	if err := markJourneyReset(tx, userID); err != nil {
		return err
	}
	return dropBadges(tx, userID,
		api.FirstSession, api.TwentySessions, api.HundredSessions,
		api.FirstDeepSession, api.AllThemesExplored,
		api.ThreeDayStreak, api.SevenDayStreak, api.ThirtyDayStreak)
}
