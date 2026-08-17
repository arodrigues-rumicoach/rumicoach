// Package badge owns the profile-badge evaluation: which badges a user has earned and
// the moment they earn them. It exists as a service (rather than logic inside the
// profile handler, where it started) so awarding can run at the moment of achievement —
// session end — and not only when the user happens to open the Profile tab. EarnedAt is
// an analytics timestamp: "when did users typically reach their first deep session?"
// only means something if the row is written then, not days later on a profile visit.
package badge

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"go.uber.org/zap"
)

// deepSessionTypes are the themed deep sessions that count toward firstDeepSession and
// allThemesExplored. The opening pair (onboarding intro, Vision) is deliberately
// excluded: those are the doorway, not the journey.
var deepSessionTypes = []string{
	string(api.SessionTypeSessionMovement),
	string(api.SessionTypeSessionValues),
	string(api.SessionTypeSessionEnergy),
	string(api.SessionTypeSessionDecisions),
	string(api.SessionTypeSessionBeliefs),
	string(api.SessionTypeSessionIdentity),
	string(api.SessionTypeSessionAcceptance),
	string(api.SessionTypeSessionPriorities),
}

// EvaluateAndAward computes every badge condition for the user and persists any newly
// earned badges to user_badges. Awarded rows are never revoked — conditions are
// re-checked only to find NEW badges (a broken streak does not take sevenDayStreak
// away). Returns the newly awarded badges. Errors on individual queries degrade to
// "condition not met" rather than failing the whole evaluation: a badge granted a
// visit late is better than a profile request failing.
func EvaluateAndAward(userID string, loc *time.Location, logger *zap.Logger) []models.UserBadge {
	var existing []models.UserBadge
	database.DB.Where("user_id = ?", userID).Find(&existing)

	earned := make(map[string]bool, len(existing))
	for _, b := range existing {
		earned[b.BadgeType] = true
	}

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		logger.Error("badge evaluation: user not found", zap.String("user_id", userID), zap.Error(err))
		return nil
	}

	// --- Inputs, each self-contained so the service can run from any call site ---

	longestStreak, doneSessions := sessionStats(userID, loc)

	var oneTimeKept, behaviorKept int64
	database.DB.Model(&models.Commitment{}).Where("user_id = ? AND done = ?", userID, true).Count(&oneTimeKept)
	database.DB.Model(&models.BehaviorCheckIn{}).Where("user_id = ? AND status = ?", userID, models.BehaviorCheckInKept).Count(&behaviorKept)
	commitmentsKept := oneTimeKept + behaviorKept

	var insights int64
	database.DB.Model(&models.UserMemory{}).Where("user_id = ? AND category = ?", userID, "insight").Count(&insights)

	var deepSessions int64
	models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, user.JourneyResetAt).
		Where("end_time IS NOT NULL AND session_type IN ?", deepSessionTypes).
		Count(&deepSessions)

	var themesExplored int64
	models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, user.JourneyResetAt).
		Where("end_time IS NOT NULL AND session_type IN ?", deepSessionTypes).
		Distinct("session_type").Count(&themesExplored)

	var activeIntegrations int64
	database.DB.Model(&models.Integration{}).
		Where("user_id = ? AND status = ?", userID, models.IntegrationActive).
		Count(&activeIntegrations)

	var wheels []models.WheelOfLifeExercise
	database.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&wheels)
	hasWheel := len(wheels) > 0

	// wheelRemapped: the user re-took the wheel at least 30 days after first mapping it.
	wheelRemapped := len(wheels) >= 2 &&
		wheels[len(wheels)-1].CreatedAt.Sub(wheels[0].CreatedAt) >= 30*24*time.Hour

	// areaImproved: any area scored at least 2 points higher in the LATEST wheel than in
	// the FIRST — the whole journey, not consecutive pairs, so one bad re-score cannot
	// lose it. Area names are the stored, user-language names, matched exactly.
	areaImproved := false
	if len(wheels) >= 2 {
		type wheelItem struct {
			Name         string  `json:"name"`
			CurrentScore float64 `json:"currentScore"`
		}
		var first, latest []wheelItem
		if json.Unmarshal([]byte(wheels[0].Data), &first) == nil &&
			json.Unmarshal([]byte(wheels[len(wheels)-1].Data), &latest) == nil {
			baseline := make(map[string]float64, len(first))
			for _, it := range first {
				baseline[it.Name] = it.CurrentScore
			}
			for _, it := range latest {
				if b, ok := baseline[it.Name]; ok && b > 0 && it.CurrentScore-b >= 2 {
					areaImproved = true
					break
				}
			}
		}
	}

	// --- Conditions ---

	type award struct {
		badge api.BadgeType
		met   bool
	}
	// Ordered as the app's 3x5 grid reads, so this slice and the tiles stay legible
	// side by side. Order has no effect on awarding: every condition is checked.
	conditions := []award{
		{api.FirstSession, doneSessions >= 1},
		// visionSet requires the mapped wheel too, matching the badge copy: "You mapped
		// your ideal life — and where you stand today."
		{api.VisionSet, user.IdealLifeVision != nil && hasWheel},
		{api.FirstCommitment, commitmentsKept >= 1},
		{api.FirstDeepSession, deepSessions >= 1},
		{api.AlwaysWithYou, activeIntegrations >= 1},
		{api.ThreeDayStreak, longestStreak >= 3},
		{api.TenInsights, insights >= 10},
		{api.AllThemesExplored, themesExplored >= int64(len(deepSessionTypes))},
		{api.SevenDayStreak, longestStreak >= 7},
		{api.TwentySessions, doneSessions >= 20},
		{api.TwentyFiveCommitments, commitmentsKept >= 25},
		{api.ThirtyDayStreak, longestStreak >= 30},
		{api.WheelRemapped, wheelRemapped},
		{api.AreaImproved, areaImproved},
		{api.HundredSessions, doneSessions >= 100},
	}

	var awarded []models.UserBadge
	for _, c := range conditions {
		if !c.met || earned[string(c.badge)] {
			continue
		}
		b := models.UserBadge{UserID: userID, BadgeType: string(c.badge), EarnedAt: time.Now()}
		if err := database.DB.Create(&b).Error; err != nil {
			logger.Error("failed to award badge", zap.String("badge", string(c.badge)), zap.Error(err))
			continue
		}
		earned[string(c.badge)] = true
		awarded = append(awarded, b)
		logger.Info("badge awarded", zap.String("user_id", userID), zap.String("badge", string(c.badge)))
	}
	return awarded
}

// sessionStats returns, from one pass over the user's session history:
//
//   - longest: the longest run of consecutive session days — the same definition the
//     /streak endpoint uses. Days are bucketed in loc (nil = UTC), matching how the user
//     experiences their own calendar.
//   - done: how many sessions actually count as sessions, feeding the firstSession →
//     twentySessions → hundredSessions ladder.
//
// Both apply journey.SessionCountsAsDone, so a connection the user opened and abandoned
// after ten seconds neither extends a streak nor buys a rung on the ladder. That makes
// `done` deliberately stricter than the totalSessions figure on the profile's progress
// grid, which counts every row.
func sessionStats(userID string, loc *time.Location) (longest int, done int) {
	if loc == nil {
		loc = time.UTC
	}
	type sessionRecord struct {
		SessionType *string
		EndTime     *time.Time
		Duration    int
	}
	var records []sessionRecord
	if err := models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, models.JourneyStartFor(database.DB, userID)).
		Select("session_type, end_time, duration").
		Where("end_time IS NOT NULL").
		Order("end_time asc").
		Scan(&records).Error; err != nil {
		return 0, 0
	}
	seen := make(map[string]struct{}, len(records))
	var keys []string
	for _, r := range records {
		sessionType := ""
		if r.SessionType != nil {
			sessionType = *r.SessionType
		}
		if r.EndTime == nil || !journey.SessionCountsAsDone(sessionType, r.Duration) {
			continue
		}
		done++
		key := r.EndTime.In(loc).Format("2006-01-02")
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var days []time.Time
	for _, k := range keys {
		d, _ := time.Parse("2006-01-02", k)
		days = append(days, d)
	}
	run := 0
	for i, d := range days {
		if i > 0 && d.Equal(days[i-1].AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}
	return longest, done
}
