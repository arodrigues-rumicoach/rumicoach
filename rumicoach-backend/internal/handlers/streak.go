package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// GetStreak implements api.ServerInterface.
// Returns the current streak, longest streak, total open days, and a list of
// dates (YYYY-MM-DD) on which the user opened the app.
func (s *Server) GetStreak(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	currentStreak, longestStreak, totalDays, calendarDates, err := s.CalculateUserStreak(userID, GetTimezoneLocation(r))
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch streak"}`, http.StatusInternalServerError)
		return
	}

	resp := api.StreakResponse{
		CurrentStreak: &currentStreak,
		LongestStreak: &longestStreak,
		TotalDays:     &totalDays,
		Calendar:      &calendarDates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// CalculateUserStreak computes streak statistics for the given user.
//
// A streak day is a local calendar day on which the user actually DID a session —
// one that ended and was long enough to count as done (journey.SessionCountsAsDone,
// the product-wide definition). Merely opening the app no longer counts: app opens
// are still recorded on GET /me for analytics, but streaks stopped reading them.
func (s *Server) CalculateUserStreak(userID string, loc *time.Location) (int, int, int, []openapi_types.Date, error) {
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
		s.logger.Error("failed to fetch streak data", zap.String("user_id", userID), zap.Error(err))
		return 0, 0, 0, nil, err
	}

	// Build a sorted, deduplicated list of the local days a qualifying session ended on.
	dateSet := make(map[string]struct{}, len(records))
	for _, rec := range records {
		sessionType := ""
		if rec.SessionType != nil {
			sessionType = *rec.SessionType
		}
		if rec.EndTime == nil || !journey.SessionCountsAsDone(sessionType, rec.Duration) {
			continue
		}
		dateSet[rec.EndTime.In(loc).Format("2006-01-02")] = struct{}{}
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	totalDays := len(dates)

	currentStreak := 0
	longestStreak := 0

	if totalDays > 0 {
		nowLocal := time.Now().In(loc)
		today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)
		expected := today

		// Walk backwards from today to count the current streak.
		for i := totalDays - 1; i >= 0; i-- {
			d, _ := time.Parse("2006-01-02", dates[i])
			if d.Equal(expected) {
				currentStreak++
				expected = expected.AddDate(0, 0, -1)
			} else if d.Before(expected) {
				break
			}
		}

		// If today hasn't been opened yet, check if a streak ending yesterday exists.
		if currentStreak == 0 {
			expected = today.AddDate(0, 0, -1)
			for i := totalDays - 1; i >= 0; i-- {
				d, _ := time.Parse("2006-01-02", dates[i])
				if d.Equal(expected) {
					currentStreak++
					expected = expected.AddDate(0, 0, -1)
				} else if d.Before(expected) {
					break
				}
			}
		}

		// Longest streak — scan all dates in chronological order.
		run := 1
		for i := 1; i < totalDays; i++ {
			prev, _ := time.Parse("2006-01-02", dates[i-1])
			curr, _ := time.Parse("2006-01-02", dates[i])
			if curr.Equal(prev.AddDate(0, 0, 1)) {
				run++
			} else {
				if run > longestStreak {
					longestStreak = run
				}
				run = 1
			}
		}
		if run > longestStreak {
			longestStreak = run
		}
	}

	var calendarDates []openapi_types.Date
	if totalDays > 0 {
		calendarDates = make([]openapi_types.Date, len(dates))
		for i, d := range dates {
			t, _ := time.Parse("2006-01-02", d)
			calendarDates[i] = openapi_types.Date{Time: t}
		}
	}

	return currentStreak, longestStreak, totalDays, calendarDates, nil
}

// GetUsageCalendar implements api.ServerInterface.
func (s *Server) GetUsageCalendar(w http.ResponseWriter, r *http.Request, params api.GetUsageCalendarParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	monthTime, err := time.Parse("2006-01", params.Month)
	if err != nil {
		http.Error(w, `{"error": "Invalid month format. Expected YYYY-MM"}`, http.StatusBadRequest)
		return
	}

	loc := GetTimezoneLocation(r)
	startOfMonth := time.Date(monthTime.Year(), monthTime.Month(), 1, 0, 0, 0, 0, loc).UTC()
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Nanosecond).UTC()

	currentStreak, _, _, calendarDates, err := s.CalculateUserStreak(userID, loc)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch streak"}`, http.StatusInternalServerError)
		return
	}

	openDays := make(map[string]bool)
	for _, d := range calendarDates {
		openDays[d.Time.Format("2006-01-02")] = true
	}

	var dbSessions []models.CommunicationSession
	if err := database.DB.
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, startOfMonth, endOfMonth).
		Order("start_time asc").
		Find(&dbSessions).Error; err != nil {
		s.logger.Error("failed to fetch sessions for usage calendar", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "Failed to fetch sessions"}`, http.StatusInternalServerError)
		return
	}

	var totalSeconds int
	sessionMap := make(map[string][]api.CommunicationSession)

	for _, dbS := range dbSessions {
		if dbS.EndTime != nil && true {
			dur := int(dbS.EndTime.Sub(dbS.StartTime).Seconds())
			totalSeconds += dur
		}

		apiS := api.CommunicationSession{
			Id:          &dbS.ID,
			SessionType: dbS.SessionType,
			StartTime:   &dbS.StartTime,
			RecapTitle:  dbS.RecapTitle,
			Recap:       dbS.Recap,
		}

		if dbS.EndTime != nil && true {
			dur := int(dbS.EndTime.Sub(dbS.StartTime).Seconds())
			apiS.Duration = &dur
		}

		if dbS.UserEvaluation != nil {
			eval := float32(*dbS.UserEvaluation)
			apiS.UserEvaluation = &eval
		}
		if dbS.UserSessionInsight != nil {
			apiS.UserSessionInsight = dbS.UserSessionInsight
		}
		if dbS.UserFeedback != nil {
			apiS.UserFeedback = dbS.UserFeedback
		}

		localDate := dbS.StartTime.In(loc).Format("2006-01-02")
		sessionMap[localDate] = append(sessionMap[localDate], apiS)
	}

	daysResponse := make(map[string]struct {
		Date     *openapi_types.Date                `json:"date,omitempty"`
		Kind     *api.UsageCalendarResponseDaysKind `json:"kind,omitempty"`
		Sessions *[]api.CommunicationSession        `json:"sessions,omitempty"`
	})

	for d := 1; d <= 31; d++ {
		dateObj := time.Date(monthTime.Year(), monthTime.Month(), d, 0, 0, 0, 0, loc)
		if dateObj.Month() != monthTime.Month() {
			break
		}
		dateStr := dateObj.Format("2006-01-02")

		sessions := sessionMap[dateStr]

		kindStr := api.UsageCalendarResponseDaysKindNone
		if len(sessions) > 0 {
			kindStr = api.UsageCalendarResponseDaysKindSession
		} else if openDays[dateStr] {
			kindStr = api.UsageCalendarResponseDaysKindCheckin
		}

		openApiDate := openapi_types.Date{Time: dateObj}

		daysResponse[dateStr] = struct {
			Date     *openapi_types.Date                `json:"date,omitempty"`
			Kind     *api.UsageCalendarResponseDaysKind `json:"kind,omitempty"`
			Sessions *[]api.CommunicationSession        `json:"sessions,omitempty"`
		}{
			Date:     &openApiDate,
			Kind:     &kindStr,
			Sessions: &sessions,
		}
	}

	hours := float32(totalSeconds) / 3600.0
	sessionsCount := len(dbSessions)

	resp := api.UsageCalendarResponse{
		DayStreak:     &currentStreak,
		Hours:         &hours,
		SessionsCount: &sessionsCount,
		Days:          &daysResponse,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
