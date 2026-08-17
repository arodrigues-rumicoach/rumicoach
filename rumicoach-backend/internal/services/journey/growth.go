// Package growth holds shared logic about a user's journey — chiefly which coaching
// session is planned for them. It reads the persisted DailyJourney snapshot (written by the
// journey endpoint) and the session history, so both the HTTP layer and the chat runtime can
// ask "what session is planned today?" without duplicating the rule.
package journey

import (
	"sort"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
)

// deepSequence is the order of the deep developmental coaching sessions. The first entry
// is the opening Vision session (the onboarding intro precedes this but is not proposed
// on the Journey screen). Vision is never revisited when the journey cycles.
var deepSequence = []string{
	string(api.SessionTypeSessionVision),
	string(api.SessionTypeSessionMovement),
	string(api.SessionTypeSessionValues),
	string(api.SessionTypeSessionEnergy),
	string(api.SessionTypeSessionDecisions),
	string(api.SessionTypeSessionBeliefs),
	string(api.SessionTypeSessionIdentity),
	string(api.SessionTypeSessionAcceptance),
	string(api.SessionTypeSessionPriorities),
}

// minDoneSessionDuration is the minimum length for a coaching session to count as
// actually done when deciding what to propose next. Real sessions run 10+ minutes;
// anything shorter is an abandoned start (the user connected and left almost
// immediately) and must not mark the session type as completed.
const minDoneSessionDuration = 5 * time.Minute

// minDoneIntroDuration is the same threshold for the onboarding intro, which is short by
// design — a greeting, the privacy explanation, a roadmap and one question. Holding the
// intro to the full-session threshold would mean a properly completed one never
// registers, and the user would be routed back through onboarding forever.
const minDoneIntroDuration = 45 * time.Second

// SessionCountsAsDone reports whether a communication session was a real, completed
// session rather than an abandoned start. A zero duration (still running, or a crashed
// connection) never counts. Exported because it is the product-wide definition of
// "the user did a session" — the journey gates, the streak calculation and the badge
// service must all agree on it.
func SessionCountsAsDone(sessionType string, duration int) bool {
	threshold := minDoneSessionDuration
	if sessionType == string(api.SessionTypeOnboarding) {
		threshold = minDoneIntroDuration
	}
	return time.Duration(duration)*time.Second >= threshold
}

// ProposeSession computes which coaching session to propose to the user right now. Users still
// in an onboarding state are routed back to onboarding; otherwise it proposes the next deep
// developmental session from the user's history — Movement the day after onboarding, then one
// new deep session every 5 days — and falls back to the daily check-in when none is due.
// Gates are calendar-day based in loc (hours never matter); a nil loc defaults to UTC.
func ProposeSession(user *models.User, loc *time.Location) api.SessionType {
	if user == nil {
		return api.SessionTypeCheckin
	}

	// 1. The opening pair, judged by what it has produced rather than by where
	// users.state happens to be parked. The intro exists to collect the profile
	// details and Vision exists to write the ideal-life vision; while either is
	// missing, that is what the user is here to do, whatever the session history says.
	//
	// This used to read users.state, which could not tell "never did the intro" from
	// "did the intro and declined to continue" — VISION_IDEAL_LIFE is the default on a
	// fresh account — and so leant on session history to disambiguate. The artifacts
	// answer directly: they appear only when the work is actually done, and no
	// abandoned start can fake one.
	if user.NeedsProfileDetails() || user.IdealLifeVisionSetAt == nil {
		return api.SessionTypeSessionVision
	}

	// 2. Otherwise history decides: the next deep session when it is already available,
	// else the daily check-in.
	nextDeepSession, availableAt := deepSessionProgress(user.ID, loc)
	if nextDeepSession != "" && !time.Now().Before(availableAt) {
		return api.SessionType(nextDeepSession)
	}
	return api.SessionTypeCheckin
}

// deepSessionProgress computes the next not-yet-completed deep session for a user and the
// time it becomes available. Only sessions with real engagement count as done — a session
// the user abandoned right after the greeting ("something came up, I'll come back later")
// must not consume the deep-session slot, otherwise they return the same day and are
// silently rerouted to a plain check-in. Gates are calendar days in loc — hours never
// matter: Movement unlocks the day after onboarding (next local midnight), every
// subsequent deep session 5 days after the day the last deep session was done.
func deepSessionProgress(userID string, loc *time.Location) (nextDeepSession string, availableAt time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	type sessionRecord struct {
		SessionType string
		StartTime   time.Time
		Duration    int
	}
	var records []sessionRecord
	models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, models.JourneyStartFor(database.DB, userID)).
		Select("session_type, start_time, duration").
		Where("session_type IS NOT NULL").
		Scan(&records)

	lastDone := make(map[string]time.Time, len(records))
	for _, r := range records {
		if !SessionCountsAsDone(r.SessionType, r.Duration) {
			continue
		}
		if r.StartTime.After(lastDone[r.SessionType]) {
			lastDone[r.SessionType] = r.StartTime
		}
	}

	// The most recent time any deep session was done, and the next one not yet completed.
	var lastDeepTime time.Time
	for _, ds := range deepSequence {
		if t, ok := lastDone[ds]; ok {
			if t.After(lastDeepTime) {
				lastDeepTime = t
			}
		} else if nextDeepSession == "" {
			nextDeepSession = ds
		}
	}

	if nextDeepSession == "" {
		// Journey complete: cycle. The path never dead-ends — revisit the least
		// recently done deep session (never onboarding), 5 days after the most
		// recent one, so there is always a next chapter.
		var oldest string
		var oldestAt time.Time
		// Skip the opening pair (Vision): it is done once and
		// never comes around again.
		for _, ds := range deepSequence[1:] {
			if t := lastDone[ds]; oldest == "" || t.Before(oldestAt) {
				oldest = ds
				oldestAt = t
			}
		}
		if oldest == "" {
			return "", time.Time{}
		}
		return oldest, startOfDay(lastDeepTime, loc).AddDate(0, 0, 5)
	}
	if lastDeepTime.IsZero() {
		return nextDeepSession, time.Now()
	}
	// Vision follows the intro with no wait at all: the intro is a couple of minutes,
	// and the two are one continuous first meeting — making the user sleep on it before
	// the exercise that gives the journey its point would be absurd.
	if nextDeepSession == string(api.SessionTypeSessionVision) {
		return nextDeepSession, time.Now()
	}
	// Movement unlocks the day after Vision (the first substantial session); later deep
	// sessions wait 5 days.
	if nextDeepSession == string(api.SessionTypeSessionMovement) {
		return nextDeepSession, startOfDay(lastDeepTime, loc).AddDate(0, 0, 1)
	}
	return nextDeepSession, startOfDay(lastDeepTime, loc).AddDate(0, 0, 5)
}

// startOfDay returns local midnight of t's calendar day in loc — the anchor for
// day-based gates, so the time of day a session happened never shifts the unlock.
func startOfDay(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}

// NextDeepSession returns the next deep session on the user's journey and the time it
// becomes available (which may be in the past when it is available now), so the app can
// always show what comes next. Once the first pass is complete the journey cycles through
// the deep sessions again. ok is false only while the user is still inside the opening
// pair (whichever of the intro/Vision they are in IS today's session, not a future
// chapter). Gates are calendar-day based in loc; a nil loc defaults to UTC.
func NextDeepSession(user *models.User, loc *time.Location) (session api.SessionType, availableAt time.Time, ok bool) {
	if user == nil {
		return "", time.Time{}, false
	}
	// Inside the opening pair there is no "next chapter" to advertise — whichever of the
	// intro/Vision they still owe IS today's session. Same artifact test as
	// ProposeSession, so the two cannot disagree about when the pair is over.
	if user.NeedsProfileDetails() || user.IdealLifeVisionSetAt == nil {
		return "", time.Time{}, false
	}
	next, at := deepSessionProgress(user.ID, loc)
	if next == "" || next == string(api.SessionTypeOnboarding) || next == string(api.SessionTypeSessionVision) {
		return "", time.Time{}, false
	}
	return api.SessionType(next), at, true
}

type DeepSessionInfo struct {
	Session     api.SessionType
	AvailableAt time.Time
}

// UpcomingDeepSessions returns the road ahead: every deep session the user has not yet
// done, in order, with the date each is estimated to unlock. The first entry is whatever
// is available now.
//
// Unlike NextDeepSession — which answers "what is the next FUTURE chapter?" and so stays
// silent while the user is inside the opening pair — this is the whole roadmap, and it is
// most useful precisely at the start. It therefore has no opening-pair guard: a fresh
// account (which now defaults to VISION_IDEAL_LIFE) used to get an empty list, so the
// Journey screen had no roadmap to show for exactly the users who needed one most.
func UpcomingDeepSessions(user *models.User, loc *time.Location) []DeepSessionInfo {
	if user == nil {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}

	type sessionRecord struct {
		SessionType string
		StartTime   time.Time
		Duration    int
	}
	var records []sessionRecord
	models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), user.ID, user.JourneyResetAt).
		Select("session_type, start_time, duration").
		Where("session_type IS NOT NULL").
		Scan(&records)

	lastDone := make(map[string]time.Time, len(records))
	for _, r := range records {
		if !SessionCountsAsDone(r.SessionType, r.Duration) {
			continue
		}
		if r.StartTime.After(lastDone[r.SessionType]) {
			lastDone[r.SessionType] = r.StartTime
		}
	}

	var lastDeepTime time.Time
	var remaining []string

	for _, ds := range deepSequence {
		if t, ok := lastDone[ds]; ok {
			if t.After(lastDeepTime) {
				lastDeepTime = t
			}
		} else {
			// Vision belongs in the list like any other: it is the first thing ahead of
			// a new user, and dropping it left the roadmap starting at Movement with an
			// "available now" date that was simply untrue — Movement only unlocks the
			// day after Vision is done.
			remaining = append(remaining, ds)
		}
	}

	if len(remaining) == 0 {
		type cycledSession struct {
			SessionType string
			DoneAt      time.Time
		}
		var cycled []cycledSession
		for _, ds := range deepSequence[1:] {
			cycled = append(cycled, cycledSession{
				SessionType: ds,
				DoneAt:      lastDone[ds],
			})
		}
		sort.Slice(cycled, func(i, j int) bool {
			return cycled[i].DoneAt.Before(cycled[j].DoneAt)
		})
		for _, c := range cycled {
			remaining = append(remaining, c.SessionType)
		}
	}

	if len(remaining) == 0 {
		return nil
	}

	now := time.Now().In(loc)
	nowStart := startOfDay(now, loc)

	result := make([]DeepSessionInfo, 0, len(remaining))
	var currentEstCompletion time.Time

	for i, ds := range remaining {
		var avail time.Time
		if i == 0 {
			if lastDeepTime.IsZero() {
				avail = now
			} else if ds == string(api.SessionTypeSessionMovement) {
				avail = startOfDay(lastDeepTime, loc).AddDate(0, 0, 1)
			} else {
				avail = startOfDay(lastDeepTime, loc).AddDate(0, 0, 5)
			}
		} else {
			if ds == string(api.SessionTypeSessionMovement) {
				avail = currentEstCompletion.AddDate(0, 0, 1)
			} else {
				avail = currentEstCompletion.AddDate(0, 0, 5)
			}
		}

		availStart := startOfDay(avail, loc)
		if availStart.Before(nowStart) {
			currentEstCompletion = nowStart
		} else {
			currentEstCompletion = availStart
		}

		result = append(result, DeepSessionInfo{
			Session:     api.SessionType(ds),
			AvailableAt: avail,
		})
	}

	return result
}

// PlannedSessionForToday returns the deep session proposed for the user today (from the
// DailyJourney snapshot the journey endpoint persists) when it has NOT yet been done today. It
// returns "" when nothing is planned — i.e. there is no snapshot for today, the proposal is the
// default daily check-in, or the proposed session was already completed today.
//
// loc is the user's timezone (used to resolve "today"); a nil loc defaults to UTC.
func PlannedSessionForToday(userID string, loc *time.Location) api.SessionType {
	if loc == nil {
		loc = time.UTC
	}
	nowLocal := time.Now().In(loc)
	todayStr := nowLocal.Format("2006-01-02")

	var daily models.DailyJourney
	if err := database.DB.Where("user_id = ? AND date = ?", userID, todayStr).First(&daily).Error; err != nil {
		return ""
	}

	planned := api.SessionType(daily.SessionType)
	// The daily check-in is the default fallback, not a "planned" session to offer.
	if planned == "" || planned == api.SessionTypeCheckin {
		return ""
	}

	// Do not offer a session the user already completed today. Abandoned starts (the user
	// connected and left within minutes) do not count — the session should be offered again.
	todayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	var todays []models.CommunicationSession
	database.DB.Select("start_time, duration").
		Where("user_id = ? AND session_type = ? AND end_time >= ?", userID, string(planned), todayStart).
		Find(&todays)
	for _, cs := range todays {
		if SessionCountsAsDone(string(planned), cs.Duration) {
			return ""
		}
	}

	return planned
}
