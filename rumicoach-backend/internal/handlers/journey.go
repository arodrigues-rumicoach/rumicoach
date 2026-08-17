package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"github.com/rumi/rumi-be/internal/services/quote"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// GetDailyJourney implements api.ServerInterface
func (s *Server) GetDailyJourney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		s.logger.Info("user not found for a valid token; answering 404", zap.String("user_id", userID))
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	lang := "en-US"
	if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
		lang = *user.PreferredLanguage
	}

	loc := GetTimezoneLocation(r)
	nowLocal := time.Now().In(loc)
	todayStr := nowLocal.Format("2006-01-02")
	var proposedSession *api.SessionType
	var respQuote *api.Quote

	// Check if daily journey data already exists for today
	var daily models.DailyJourney
	if err := database.DB.Where("user_id = ? AND date = ?", userID, todayStr).First(&daily).Error; err == nil {
		// Daily growth exists, reuse saved information
		sessType := api.SessionType(daily.SessionType)
		proposedSession = &sessType

		// If the proposed session was already completed today, recalculate the next proposed session
		sessionCompleted := false
		if *proposedSession != api.SessionTypeCheckin {
			todayStartLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
			var count int64
			if err := models.JourneySessions(database.DB.Model(&models.CommunicationSession{}), userID, user.JourneyResetAt).
				Where("session_type = ? AND end_time >= ?", string(*proposedSession), todayStartLocal).
				Count(&count).Error; err == nil && count > 0 {
				sessionCompleted = true
			}
		}

		if sessionCompleted {
			proposedSession = nil
		}

		// Always reuse the quote and streak saved in today's DailyJourney record
		quoteText, author, quoteCategory, found := quote.GlobalQuoteService.GetQuoteByID(daily.QuoteID, lang)
		if found {
			qCat := api.QuoteCategory(quoteCategory)
			respQuote = &api.Quote{
				Id:       &daily.QuoteID,
				Quote:    &quoteText,
				Author:   author,
				Category: &qCat,
			}
		}
	}

	// Calculate and persist if missing
	if proposedSession == nil || respQuote == nil {
		if proposedSession == nil {
			val := journey.ProposeSession(&user, loc)
			proposedSession = &val
		}

		if respQuote == nil {
			// Calculate daily quote, filtered by the AI's end-of-session category
			// pick when one exists (users.journey_quote_category), unless the
			// user is still in onboarding (always "growth" quote).
			catPick := user.JourneyQuoteCategory
			if user.State != nil && models.SessionState(*user.State).IsOnboarding() {
				growthCat := "growth"
				catPick = &growthCat
			}
			quoteText, author, quoteCategory, quoteID := quote.GlobalQuoteService.GetRandomQuoteData(lang, catPick)

			qCat := api.QuoteCategory(quoteCategory)
			respQuote = &api.Quote{
				Id:       &quoteID,
				Quote:    &quoteText,
				Author:   author,
				Category: &qCat,
			}
		}
		if daily.ID == "" {
			daily = models.DailyJourney{
				UserID:      userID,
				Date:        todayStr,
				SessionType: string(*proposedSession),
				QuoteID:     *respQuote.Id,
			}

			if err := database.DB.Create(&daily).Error; err != nil {
				if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
					s.logger.Info("Daily growth record created concurrently, fetching existing record", zap.String("user_id", userID))
					if fetchErr := database.DB.Where("user_id = ? AND date = ?", userID, todayStr).First(&daily).Error; fetchErr == nil {
						sessType := api.SessionType(daily.SessionType)
						proposedSession = &sessType

						quoteText, author, quoteCategory, found := quote.GlobalQuoteService.GetQuoteByID(daily.QuoteID, lang)
						if found {
							qCat := api.QuoteCategory(quoteCategory)
							respQuote = &api.Quote{
								Id:       &daily.QuoteID,
								Quote:    &quoteText,
								Author:   author,
								Category: &qCat,
							}
						}

					} else {
						s.logger.Error("Failed to fetch concurrently created daily journey data", zap.Error(fetchErr), zap.String("user_id", userID))
					}
				} else {
					s.logger.Error("Failed to save daily journey data", zap.Error(err), zap.String("user_id", userID))
				}
			}
		} else {
			daily.SessionType = string(*proposedSession)
			daily.QuoteID = *respQuote.Id
			if err := database.DB.Save(&daily).Error; err != nil {
				s.logger.Error("Failed to update daily journey data", zap.Error(err), zap.String("user_id", userID))
			}
		}
	}

	// `session` is what the user can do TODAY, and the growth carousel renders it as its
	// first card. Withholding the check-in here left that first position empty on every
	// day between deep sessions — which is most days — so the screen opened on a session
	// the user cannot start yet instead of the one thing that is always available. The
	// check-in is a real proposal (it is already persisted as today's DailyJourney type);
	// expose it like any other.
	respSession := proposedSession

	// Theme is only present when the AI's end-of-session pick (journey_theme) is
	// active — i.e. when it should override whatever the client currently shows.
	// The user's manual choice lives on /me (and is cleared server-side into
	// precedence by PATCH /me wiping journey_theme); echoing it here as a fallback
	// caused a race where a concurrent /journey fetch re-applied a stale
	// theme right after the user picked a new one.
	var theme *string
	if user.JourneyTheme != nil && *user.JourneyTheme != "" {
		theme = user.JourneyTheme
	}

	// Build and write response
	modeVal := determineSessionMode(&user, proposedSession)
	resp := api.JourneySessionResponse{
		Session:     respSession,
		Mode:        &modeVal,
		Quote:       respQuote,
		Theme:       theme,
		NextSession: nextSessionInfo(&user, loc),
		Sessions:    nextSessionsInfo(&user, loc),
		FocusArea:   user.FocusArea,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// nextSessionInfo resolves the next deep session on the user's journey for the growth
// payload, so the app can always preview the upcoming chapter and when it unlocks.
// loc is the user's timezone: journey gates are calendar-day based.
func nextSessionInfo(user *models.User, loc *time.Location) *api.SessionInfo {
	next, availableAt, ok := journey.NextDeepSession(user, loc)
	if !ok {
		return nil
	}
	return &api.SessionInfo{Session: next, AvailableAt: availableAt}
}

// nextSessionsInfo resolves all upcoming deep sessions on the user's journey and their
// estimated availability dates.
func nextSessionsInfo(user *models.User, loc *time.Location) *[]api.SessionInfo {
	upcoming := journey.UpcomingDeepSessions(user, loc)
	if len(upcoming) == 0 {
		return nil
	}
	res := make([]api.SessionInfo, len(upcoming))
	for i, s := range upcoming {
		res[i] = api.SessionInfo{
			Session:     s.Session,
			AvailableAt: s.AvailableAt,
		}
	}
	return &res
}

func determineSessionMode(user *models.User, proposedSession *api.SessionType) api.JourneySessionResponseMode {
	if proposedSession == nil {
		return api.Start
	}

	if *proposedSession == api.SessionTypeOnboarding {
		if user.State != nil && *user.State != string(models.StateOnboardingIntro) {
			return api.Resume
		}
		return api.Start
	}

	// For other sessions, check if the user has a recent active session (less than 2 hours old)
	if user.LatestSessionHandle != nil && user.LatestSessionHandleAt != nil {
		if time.Since(*user.LatestSessionHandleAt) < 2*time.Hour {
			return api.Resume
		}
	}

	return api.Start
}
