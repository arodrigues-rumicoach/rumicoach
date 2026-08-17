package companion

import (
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"go.uber.org/zap"
)

// recentSessionsInContext bounds how much session history goes into the prompt. Enough
// to reference "the session where you..." without turning the system instruction into a
// transcript archive.
const recentSessionsInContext = 5

// buildJourneyContext renders what the companion needs to answer the questions users
// actually ask it — "what's my next session?", "do I have any commitments?", "what do
// you know about me?". None of it was in the prompt before, so the coach truthfully
// answered "I don't see anything", which reads as amnesia to a user who has a whole
// journey in the app.
//
// Everything here is read-only and derived from the same sources the app shows, so the
// two can never disagree. Failures degrade to an omitted block, never a broken prompt.
func buildJourneyContext(user *models.User, loc *time.Location, logger *zap.Logger) string {
	if user == nil {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	var sb strings.Builder
	sb.WriteString("### 1.4 THE USER'S JOURNEY IN THE APP\n")
	sb.WriteString("This is the user's current state inside the Rumi app. It is the truth: when they ask what is next, what they committed to, or what you know about them, answer from THIS — never say you cannot see their plan.\n")

	// The Current Focus — the single area the user chose to work on. The most
	// referenced fact in coaching conversations and previously absent entirely.
	if user.FocusArea != nil && *user.FocusArea != "" {
		sb.WriteString(fmt.Sprintf("- Current Focus (the life area they chose to work on): %s\n", *user.FocusArea))
	} else {
		sb.WriteString("- Current Focus: not chosen yet (it is set during the Vision session).\n")
	}

	// Where they are on the journey, and what comes next.
	next, availableAt, ok := journey.NextDeepSession(user, loc)
	switch {
	case !ok:
		sb.WriteString("- Next session: they are still in their opening sessions (the intro and the Ideal Life Vision). Encourage them to open the app and continue — that is what comes next for them.\n")
	case time.Now().Before(availableAt):
		sb.WriteString(fmt.Sprintf("- Next session: %s, unlocking on %s. If they ask, tell them what it is about and when it opens — do not invent a different date.\n",
			sessionDisplayName(string(next)), availableAt.In(loc).Format("2006-01-02")))
	default:
		sb.WriteString(fmt.Sprintf("- Next session: %s, and it is AVAILABLE NOW. If the moment fits, warmly invite them to open the app and do it.\n",
			sessionDisplayName(string(next))))
	}

	// Streak: session days, matching what the app shows them.
	if streak := currentSessionStreak(user.ID, loc); streak > 0 {
		sb.WriteString(fmt.Sprintf("- Current streak: %d day(s) with a session. Worth acknowledging if it comes up naturally; never nag about it.\n", streak))
	}

	// Recent sessions, using the short recaps generated at session end. This is what
	// lets the companion say "in your last session you realised X" instead of guessing.
	var sessions []models.CommunicationSession
	if err := models.JourneySessions(database.DB, user.ID, user.JourneyResetAt).
		Select("session_type, start_time, end_time, duration, recap_title, recap, user_session_insight").
		Where("end_time IS NOT NULL").
		Order("start_time desc").Limit(recentSessionsInContext).
		Find(&sessions).Error; err != nil {
		logger.Warn("companion: failed to load recent sessions", zap.Error(err))
	}

	var done []models.CommunicationSession
	for _, s := range sessions {
		sessionType := ""
		if s.SessionType != nil {
			sessionType = *s.SessionType
		}
		if journey.SessionCountsAsDone(sessionType, s.Duration) {
			done = append(done, s)
		}
	}

	if len(done) == 0 {
		sb.WriteString("- Past sessions: none completed yet. Do not refer to sessions they have not had.\n")
		return sb.String()
	}

	sb.WriteString("- Recent sessions (most recent first). Reference these naturally when relevant; they are what the user remembers doing:\n")
	for _, s := range done {
		label := sessionDisplayName("")
		if s.SessionType != nil {
			label = sessionDisplayName(*s.SessionType)
		}
		line := fmt.Sprintf("  - %s — %s", s.StartTime.In(loc).Format("2006-01-02"), label)
		if s.RecapTitle != nil && *s.RecapTitle != "" {
			line += fmt.Sprintf(": %s", *s.RecapTitle)
		}
		sb.WriteString(line + "\n")
		if s.Recap != nil && *s.Recap != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", *s.Recap))
		} else if s.UserSessionInsight != nil && *s.UserSessionInsight != "" {
			sb.WriteString(fmt.Sprintf("    Their words: %s\n", *s.UserSessionInsight))
		}
	}
	return sb.String()
}

// sessionDisplayName maps a session type to a plain name the coach can say out loud.
// Kept here rather than reusing the chat package's version so the companion never
// speaks about "the onboarding" or other internal vocabulary.
func sessionDisplayName(sessionType string) string {
	switch sessionType {
	case "onboarding":
		return "your first conversation"
	case "session_vision":
		return "the Ideal Life Vision"
	case "session_movement":
		return "Obstacles and Movement"
	case "session_values":
		return "Values"
	case "session_energy":
		return "Energy and Vitality"
	case "session_decisions":
		return "Fears and Decisions"
	case "session_beliefs":
		return "Beliefs"
	case "checkin":
		return "a daily check-in"
	default:
		return "a session"
	}
}

// currentSessionStreak counts consecutive local days ending today (or yesterday, when
// today has no session yet) on which a qualifying session happened — the same rule the
// app's streak uses, so the two never disagree in front of the user.
func currentSessionStreak(userID string, loc *time.Location) int {
	type record struct {
		SessionType *string
		EndTime     *time.Time
		Duration    int
	}
	var records []record
	if err := models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, models.JourneyStartFor(database.DB, userID)).
		Select("session_type, end_time, duration").
		Where("end_time IS NOT NULL").
		Scan(&records).Error; err != nil {
		return 0
	}
	days := make(map[string]struct{}, len(records))
	for _, r := range records {
		sessionType := ""
		if r.SessionType != nil {
			sessionType = *r.SessionType
		}
		if r.EndTime == nil || !journey.SessionCountsAsDone(sessionType, r.Duration) {
			continue
		}
		days[r.EndTime.In(loc).Format("2006-01-02")] = struct{}{}
	}
	if len(days) == 0 {
		return 0
	}

	nowLocal := time.Now().In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	start := today
	if _, ok := days[today.Format("2006-01-02")]; !ok {
		// A streak that is still alive can end yesterday — today simply has not happened yet.
		start = today.AddDate(0, 0, -1)
	}
	streak := 0
	for d := start; ; d = d.AddDate(0, 0, -1) {
		if _, ok := days[d.Format("2006-01-02")]; !ok {
			break
		}
		streak++
	}
	return streak
}

// habitTrackingDays is how far back the recurring-commitment history goes. A week is
// what a coach would actually reference ("you've done it four times this week");
// further back turns into a spreadsheet nobody reads.
const habitTrackingDays = 7

// buildAccountabilityContext renders how the user's RECURRING commitments have actually
// been going day by day. The commitments block in the shared context shows each one's
// master done flag, which for a recurring habit means almost nothing — the daily truth
// lives in commitment_completions. Without this the companion cannot tell whether the
// morning walk happened today, which is precisely the accountability a between-sessions
// coach exists for.
func buildAccountabilityContext(user *models.User, loc *time.Location, logger *zap.Logger) string {
	if user == nil {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}

	var recurring []models.Commitment
	if err := database.DB.
		Where("user_id = ? AND type = ?", user.ID, "recurring").
		Order("created_at asc").Find(&recurring).Error; err != nil {
		logger.Warn("companion: failed to load recurring commitments", zap.Error(err))
		return ""
	}
	if len(recurring) == 0 {
		return ""
	}

	nowLocal := time.Now().In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	since := today.AddDate(0, 0, -(habitTrackingDays - 1)).Format("2006-01-02")

	var completions []models.CommitmentCompletion
	if err := database.DB.
		Where("user_id = ? AND date >= ?", user.ID, since).
		Find(&completions).Error; err != nil {
		logger.Warn("companion: failed to load commitment completions", zap.Error(err))
		return ""
	}
	doneOn := make(map[string]map[string]struct{}, len(completions))
	for _, c := range completions {
		if doneOn[c.CommitmentID] == nil {
			doneOn[c.CommitmentID] = map[string]struct{}{}
		}
		doneOn[c.CommitmentID][c.Date] = struct{}{}
	}

	todayStr := today.Format("2006-01-02")
	isoToday := int(today.Weekday())
	if isoToday == 0 {
		isoToday = 7
	}

	var sb strings.Builder
	sb.WriteString("### 1.5 HOW THEIR RECURRING COMMITMENTS ARE ACTUALLY GOING (LAST 7 DAYS)\n")
	sb.WriteString("Per-day truth for the habits they committed to. Use it to follow up specifically instead of asking vaguely how things are going. Celebrate what is happening; treat what is not as information to explore with curiosity, NEVER as failure and never as a scolding.\n")
	for _, c := range recurring {
		// Count only the days this commitment was actually scheduled for.
		scheduled, kept := 0, 0
		for i := 0; i < habitTrackingDays; i++ {
			day := today.AddDate(0, 0, -i)
			iso := int(day.Weekday())
			if iso == 0 {
				iso = 7
			}
			onThisDay := false
			for _, d := range c.Days {
				if d == iso {
					onThisDay = true
					break
				}
			}
			if !onThisDay {
				continue
			}
			scheduled++
			if _, ok := doneOn[c.ID][day.Format("2006-01-02")]; ok {
				kept++
			}
		}

		dueToday := false
		for _, d := range c.Days {
			if d == isoToday {
				dueToday = true
				break
			}
		}
		todayNote := "not scheduled for today"
		if dueToday {
			if _, ok := doneOn[c.ID][todayStr]; ok {
				todayNote = "DONE today"
			} else {
				todayNote = "due today, not done yet"
			}
		}

		// The horizon matters to how the coach talks about a habit: one ending this week
		// is something to finish and celebrate, not just another day to tick.
		horizon := ""
		if c.EndedAt != nil {
			daysLeft := int(startOfLocalDay(*c.EndedAt, loc).Sub(today).Hours() / 24)
			switch {
			case daysLeft <= 0:
				horizon = "; this was its last day"
			case daysLeft == 1:
				horizon = "; ends tomorrow"
			default:
				horizon = fmt.Sprintf("; ends in %d days (%s)", daysLeft, c.EndedAt.In(loc).Format("2006-01-02"))
			}
		}

		if scheduled == 0 {
			sb.WriteString(fmt.Sprintf("- \"%s\": %s%s.\n", c.Title, todayNote, horizon))
			continue
		}
		sb.WriteString(fmt.Sprintf("- \"%s\": kept %d of %d scheduled days this week; %s%s.\n", c.Title, kept, scheduled, todayNote, horizon))
	}
	return sb.String()
}

// foundationalMemoriesInContext caps how many older memories are surfaced.
const foundationalMemoriesInContext = 12

// buildFoundationalMemories surfaces memories OLDER than the 30-day window the shared
// context uses. The companion's long-term continuity is supposed to come from memories
// (the raw message log is purged), so a hard 30-day cut means the coach forgets who the
// user is after a month — exactly the opposite of the design. Identity, values and needs
// are the categories that stay true over time, so those come first.
func buildFoundationalMemories(user *models.User, logger *zap.Logger) string {
	if user == nil {
		return ""
	}
	cutoff := time.Now().AddDate(0, -1, 0)

	var memories []models.UserMemory
	if err := database.DB.
		Where("user_id = ? AND created_at < ? AND category IN ?", user.ID, cutoff,
			[]string{"identity", "values", "needs", "obstacles"}).
		Order("created_at desc").Limit(foundationalMemoriesInContext).
		Find(&memories).Error; err != nil {
		logger.Warn("companion: failed to load foundational memories", zap.Error(err))
		return ""
	}
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### 1.6 WHO THEY ARE (OLDER, STILL TRUE)\n")
	sb.WriteString("Lasting facts about this person, learned more than a month ago. They did not expire — treat them as current unless something recent contradicts them:\n")
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Category, m.Content))
	}
	return sb.String()
}

// startOfLocalDay returns local midnight of t's calendar day in loc.
func startOfLocalDay(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}
