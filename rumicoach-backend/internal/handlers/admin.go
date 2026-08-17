package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GetAdminSessions implements api.ServerInterface
func (s *Server) GetAdminSessions(w http.ResponseWriter, r *http.Request, params api.GetAdminSessionsParams) {
	page := 1
	if params.Page != nil {
		page = *params.Page
	}
	limit := 20
	if params.Limit != nil {
		limit = *params.Limit
	}

	var minDuration, maxDuration *float64
	if minStr := r.URL.Query().Get("minDuration"); minStr != "" {
		if val, err := strconv.ParseFloat(minStr, 64); err == nil {
			minDuration = &val
		}
	}
	if maxStr := r.URL.Query().Get("maxDuration"); maxStr != "" {
		if val, err := strconv.ParseFloat(maxStr, 64); err == nil {
			maxDuration = &val
		}
	}

	sessions, total, err := services.Admin.ListSessions(r.Context(), page, limit, params, minDuration, maxDuration)
	if err != nil {
		http.Error(w, `{"error": "Failed to list sessions"}`, http.StatusInternalServerError)
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp := api.CommunicationSessionPaginatedResponse{
		Items: &sessions,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(total)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetAdminDashboard implements api.ServerInterface: aggregate CEO/PM dashboard metrics over
// users and communication sessions, optionally scoped to a [from, to] window on the session
// start time.
func (s *Server) GetAdminDashboard(w http.ResponseWriter, r *http.Request, params api.GetAdminDashboardParams) {
	dashboard, err := services.Admin.Dashboard(r.Context(), params.From, params.To)
	if err != nil {
		s.logger.Error("Failed to build admin dashboard", zap.Error(err))
		http.Error(w, `{"error": "Failed to build dashboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard)
}

// GetAdminUsers implements api.ServerInterface
func (s *Server) GetAdminUsers(w http.ResponseWriter, r *http.Request, params api.GetAdminUsersParams) {
	page := 1
	if params.Page != nil {
		page = *params.Page
	}
	limit := 20
	if params.Limit != nil {
		limit = *params.Limit
	}

	users, total, err := services.Admin.ListUsers(r.Context(), page, limit, params)
	if err != nil {
		http.Error(w, `{"error": "Failed to list users"}`, http.StatusInternalServerError)
		return
	}

	var apiUsers []api.User
	for _, u := range users {
		var dob *openapi_types.Date
		if u.DateOfBirth != nil {
			dob = &openapi_types.Date{Time: *u.DateOfBirth}
		}

		userRef := u // capture for pointers
		var coachVoice *api.UserCoachVoice
		if u.CoachVoice != nil {
			cv := api.UserCoachVoice(*u.CoachVoice)
			coachVoice = &cv
		}

		apiUsers = append(apiUsers, api.User{
			Id:                           &u.ID,
			Email:                        u.Email,
			PhoneNumber:                  u.PhoneNumber,
			DateOfBirth:                  dob,
			Gender:                       u.Gender,
			CoachGender:                  u.CoachGender,
			CoachVoice:                   coachVoice,
			Country:                      u.Country,
			Region:                       u.Region,
			PreferredLanguage:            u.PreferredLanguage,
			IsActive:                     &userRef.IsActive,
			Waitlisted:                   &userRef.Waitlisted,
			CreatedAt:                    &userRef.CreatedAt,
			Theme:                        u.Theme,
			Name:                         u.Name,
			TermsAndConditionsAcceptedAt: u.TermsAndConditionsAcceptedAt,
			MarketingAcceptedAt:          u.MarketingAcceptedAt,
			AiAcceptedAt:                 u.AIAcceptedAt,
			LastOnlineAt:                 u.LastOnlineAt,
			LastPlatform:                 u.LastPlatform,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp := api.UserPaginatedResponse{
		Items: &apiUsers,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(total)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetAdminUsersId implements api.ServerInterface
func (s *Server) GetAdminUsersId(w http.ResponseWriter, r *http.Request, id string) {
	user, err := services.Admin.GetUser(r.Context(), id)
	if err != nil {
		s.logger.Error("Error getting user", zap.Error(err), zap.String("user_id", id))
		http.Error(w, `{"error": "Failed to get user or user not found"}`, http.StatusNotFound)
		return
	}

	var dob *openapi_types.Date
	if user.DateOfBirth != nil {
		dob = &openapi_types.Date{Time: *user.DateOfBirth}
	}

	var coachVoice *api.UserCoachVoice
	if user.CoachVoice != nil {
		cv := api.UserCoachVoice(*user.CoachVoice)
		coachVoice = &cv
	}

	apiUser := api.User{
		Id:                           &user.ID,
		Email:                        user.Email,
		PhoneNumber:                  user.PhoneNumber,
		DateOfBirth:                  dob,
		Gender:                       user.Gender,
		CoachGender:                  user.CoachGender,
		CoachVoice:                   coachVoice,
		Country:                      user.Country,
		Region:                       user.Region,
		PreferredLanguage:            user.PreferredLanguage,
		IsActive:                     &user.IsActive,
		Waitlisted:                   &user.Waitlisted,
		CreatedAt:                    &user.CreatedAt,
		Name:                         user.Name,
		TermsAndConditionsAcceptedAt: user.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          user.MarketingAcceptedAt,
		AiAcceptedAt:                 user.AIAcceptedAt,
		LastOnlineAt:                 user.LastOnlineAt,
		LastPlatform:                 user.LastPlatform,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUser)
}

// PatchAdminUsersId implements api.ServerInterface
func (s *Server) PatchAdminUsersId(w http.ResponseWriter, r *http.Request, id string) {
	var req api.PatchAdminUsersIdJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	var user *models.User
	var err error

	if req.IsActive != nil {
		user, err = services.Admin.SetUserActiveStatus(r.Context(), id, *req.IsActive)
		if err != nil {
			s.logger.Error("Error updating user active status", zap.Error(err), zap.String("user_id", id))
			http.Error(w, `{"error": "Failed to update user active status or user not found"}`, http.StatusNotFound)
			return
		}
	} else {
		user, err = services.Admin.GetUser(r.Context(), id)
		if err != nil {
			s.logger.Error("Error getting user", zap.Error(err), zap.String("user_id", id))
			http.Error(w, `{"error": "Failed to get user or user not found"}`, http.StatusNotFound)
			return
		}
	}

	var dob *openapi_types.Date
	if user.DateOfBirth != nil {
		dob = &openapi_types.Date{Time: *user.DateOfBirth}
	}

	var coachVoice *api.UserCoachVoice
	if user.CoachVoice != nil {
		cv := api.UserCoachVoice(*user.CoachVoice)
		coachVoice = &cv
	}

	apiUser := api.User{
		Id:                           &user.ID,
		Email:                        user.Email,
		PhoneNumber:                  user.PhoneNumber,
		DateOfBirth:                  dob,
		Gender:                       user.Gender,
		CoachGender:                  user.CoachGender,
		CoachVoice:                   coachVoice,
		Country:                      user.Country,
		Region:                       user.Region,
		PreferredLanguage:            user.PreferredLanguage,
		IsActive:                     &user.IsActive,
		Waitlisted:                   &user.Waitlisted,
		CreatedAt:                    &user.CreatedAt,
		Theme:                        user.Theme,
		Name:                         user.Name,
		TermsAndConditionsAcceptedAt: user.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          user.MarketingAcceptedAt,
		AiAcceptedAt:                 user.AIAcceptedAt,
		LastOnlineAt:                 user.LastOnlineAt,
		LastPlatform:                 user.LastPlatform,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUser)
}

// SetupTestData implements api.ServerInterface
func (s *Server) SetupTestData(w http.ResponseWriter, r *http.Request) {
	var req api.SetupTestDataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	sessionType := req.SessionType

	// Check if user exists
	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Clean up old user data (goals is a legacy orphaned table — purge via raw SQL if it exists)
		if tx.Migrator().HasTable("goals") {
			if err := tx.Exec("DELETE FROM goals WHERE user_id = ?", userID).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.Commitment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserMemory{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.EisenhowerMatrixExercise{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.WheelOfLifeExercise{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.DailyJourney{}).Error; err != nil {
			return err
		}
		// NOTE: real communication_sessions are intentionally NOT deleted here. Test setup
		// resets the user's state and exercise data, but genuine session history (durations,
		// costs, AI/user evaluations, insights) must be preserved for analytics and the admin
		// dashboard. Sessions seeded by a previous test-setup run carry the "test-seed-"
		// ID prefix and ARE replaced, so reruns stay idempotent and the journey
		// gates (which read typed session history) don't drift.
		// A tester who needs an account with no history at all uses POST
		// /admin/reset-account instead, which is a full account wipe rather than a persona.
		if err := tx.Exec("DELETE FROM communication_sessions WHERE user_id = ? AND id LIKE 'test-seed-%'", userID).Error; err != nil {
			return err
		}
		// Reset streak logs
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserAppOpen{}).Error; err != nil {
			return err
		}

		// Reset User attributes
		resetUserForTestSetup(&user)

		now := time.Now()

		switch sessionType {
		case api.NewUser:
			// Just clean setup, which is already done above.

		case api.OnboardingIdealVision:
			user.State = func(s string) *string { return &s }(string(models.StateVisionIdealLife))

		case api.OnboardingWheel:
			user.IdealLifeVision = func(s string) *string { return &s }("I live a balanced, peaceful life working on meaningful products with time for nature and health.")
			user.State = func(s string) *string { return &s }(string(models.StateVisionWheelOfLife))
			// Seed partial wheel of life
			wheelData := `[
				{"name": "Health", "currentScore": 6, "reasoning": "Doing okay but could sleep more"},
				{"name": "Relations", "currentScore": 0, "reasoning": ""},
				{"name": "Career", "currentScore": 0, "reasoning": ""},
				{"name": "Finances", "currentScore": 0, "reasoning": ""},
				{"name": "Growth", "currentScore": 0, "reasoning": ""}
			]`
			exercise := models.WheelOfLifeExercise{
				UserID:    userID,
				SessionID: "test-session-id-" + userID,
				Data:      wheelData,
			}
			if err := tx.Create(&exercise).Error; err != nil {
				return err
			}

		case api.OnboardingMetaphor:
			user.IdealLifeVision = func(s string) *string { return &s }("I live a balanced, peaceful life working on meaningful products with time for nature and health.")
			user.State = func(s string) *string { return &s }(string(models.StateVisionMetaphor))
			wheelData := `[
				{"name": "Health", "currentScore": 6, "reasoning": "Need more sleep"},
				{"name": "Relations", "currentScore": 7, "reasoning": "Good friends but busy"},
				{"name": "Career", "currentScore": 5, "reasoning": "Feeling stagnation"},
				{"name": "Finances", "currentScore": 8, "reasoning": "Saving well"},
				{"name": "Growth", "currentScore": 6, "reasoning": "Learning slowly"}
			]`
			exercise := models.WheelOfLifeExercise{
				UserID:    userID,
				SessionID: "test-session-id-" + userID,
				Data:      wheelData,
			}
			if err := tx.Create(&exercise).Error; err != nil {
				return err
			}

		case api.OnboardingEnding:
			user.IdealLifeVision = func(s string) *string { return &s }("I live a balanced, peaceful life working on meaningful products with time for nature and health.")
			// Onboarding emotional closing -> ending session
			user.State = func(s string) *string { return &s }(string(models.StateVisionEmotionalClosing))
			wheelData := `[
				{"name": "Health", "currentScore": 6, "reasoning": "Need more sleep"},
				{"name": "Relations", "currentScore": 7, "reasoning": "Good friends but busy"},
				{"name": "Career", "currentScore": 5, "reasoning": "Feeling stagnation"},
				{"name": "Finances", "currentScore": 8, "reasoning": "Saving well"},
				{"name": "Growth", "currentScore": 6, "reasoning": "Learning slowly"}
			]`
			exercise := models.WheelOfLifeExercise{
				UserID:    userID,
				SessionID: "test-session-id-" + userID,
				Data:      wheelData,
			}
			if err := tx.Create(&exercise).Error; err != nil {
				return err
			}
			// Set up focus area
			user.FocusArea = func(s string) *string { return &s }("Career")

		case api.CheckinDailyFirst, api.CheckinDailySameDay, api.CheckinDailyConsecutive, api.CheckinDailyAfterDays, api.CheckinDailyAfterWeeks, api.CheckinDailyAfterLong:
			// Every check-in persona is a fully furnished post-onboarding user; what
			// distinguishes them is the session history timing (streaks, gaps, journey
			// gates) and, with it, how many commitments the journey has left behind.
			// checkin_daily_first has only done onboarding; the rest also did Movement.
			stage := journeyAfterMovement
			if sessionType == api.CheckinDailyFirst {
				stage = journeyAfterOnboarding
			}
			if err := seedRealUserProfile(tx, &user, now, stage); err != nil {
				return err
			}

			switch sessionType {
			case api.CheckinDailyFirst:
				// Onboarding finished two hours ago: the very first check-in day.
				// Movement unlocks tomorrow, so today's Journey screen offers the check-in.
				if err := seedOpeningPair(tx, &user, now.Add(-2*time.Hour)); err != nil {
					return err
				}
				if err := seedAppOpens(tx, userID, now, 0); err != nil {
					return err
				}
			case api.CheckinDailySameDay:
				// Already did today's check-in 2 hours ago — tests double check-in / daily lock.
				if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -10)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -2)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeCheckin, "test-seed-checkin-1-"+userID, now.Add(-2*time.Hour)); err != nil {
					return err
				}
				if err := seedAppOpens(tx, userID, now, 0, 1, 2); err != nil {
					return err
				}
			case api.CheckinDailyConsecutive:
				// Check-ins on each of the last 3 days — streak validation. The next deep
				// session (values) is still gated (movement was 4 days ago, gate is 5).
				if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -10)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -4)); err != nil {
					return err
				}
				for i := 1; i <= 3; i++ {
					if err := seedCompletedSession(tx, &user, api.SessionTypeCheckin, fmt.Sprintf("test-seed-checkin-%d-%s", i, userID), now.AddDate(0, 0, -i).Add(-4*time.Hour)); err != nil {
						return err
					}
				}
				if err := seedAppOpens(tx, userID, now, 1, 2, 3); err != nil {
					return err
				}
			case api.CheckinDailyAfterDays:
				// Last activity 4 days ago — tests the 4-day-gap opening prompts.
				if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -14)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -4)); err != nil {
					return err
				}
				if err := seedAppOpens(tx, userID, now, 4); err != nil {
					return err
				}
			case api.CheckinDailyAfterWeeks:
				// Last activity 12 days ago — the next deep session (values) is overdue,
				// exactly like a real user coming back after two weeks away.
				if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -22)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -12)); err != nil {
					return err
				}
				if err := seedAppOpens(tx, userID, now, 12); err != nil {
					return err
				}
			case api.CheckinDailyAfterLong:
				// Last activity 35 days ago — the >1 month re-engagement flow.
				if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -45)); err != nil {
					return err
				}
				if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -35)); err != nil {
					return err
				}
				if err := seedAppOpens(tx, userID, now, 35); err != nil {
					return err
				}
			}

		case api.Stressed:
			// Full post-onboarding profile (onboarding + Movement done) plus a pile of
			// overdue commitments and a populated Eisenhower matrix — the
			// stress-mitigation coaching scenario.
			if err := seedRealUserProfile(tx, &user, now, journeyAfterMovement); err != nil {
				return err
			}
			if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -10)); err != nil {
				return err
			}
			if err := seedCompletedSession(tx, &user, api.SessionTypeSessionMovement, "test-seed-movement-"+userID, now.AddDate(0, 0, -2)); err != nil {
				return err
			}
			if err := seedAppOpens(tx, userID, now, 0, 1, 2); err != nil {
				return err
			}

			// Overdue one-time commitments (dates relative to now so the fixture stays evergreen)
			overdue := func(daysAgo int) *string {
				d := now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
				return &d
			}
			planCommitments := []models.Commitment{
				{ID: "task-1-" + userID, UserID: userID, Origin: models.CommitmentOriginPlan, Title: "Overdue important medical appointment", Type: "one_time", Done: false, Date: overdue(15)},
				{ID: "task-2-" + userID, UserID: userID, Origin: models.CommitmentOriginPlan, Title: "Prepare presentation slide deck", Type: "one_time", Done: false, Date: overdue(10)},
				{ID: "task-3-" + userID, UserID: userID, Origin: models.CommitmentOriginPlan, Title: "Review budget options", Type: "one_time", Done: false, Date: overdue(6)},
				{ID: "task-4-" + userID, UserID: userID, Origin: models.CommitmentOriginPlan, Title: "Organize filing cabinets", Type: "one_time", Done: false, Date: overdue(3)},
			}
			for i := range planCommitments {
				if err := tx.Create(&planCommitments[i]).Error; err != nil {
					return err
				}
			}

			// Matrix setup
			matrixData := `{
				"urgent_important": [
					{"task": "Overdue important medical appointment", "quadrant": "urgent_important", "reasoning": "Crucial health checkup"},
					{"task": "Prepare presentation slide deck", "quadrant": "urgent_important", "reasoning": "Presentation due tomorrow"}
				],
				"not_urgent_important": [],
				"urgent_not_important": [
					{"task": "Review budget options", "quadrant": "urgent_not_important", "reasoning": "Interruption from colleague"}
				],
				"not_urgent_not_important": [
					{"task": "Organize filing cabinets", "quadrant": "not_urgent_not_important", "reasoning": "Low priority task"}
				]
			}`
			matrix := models.EisenhowerMatrixExercise{
				UserID:    userID,
				SessionID: "stressed-session-id-" + userID,
				Data:      matrixData,
			}
			if err := tx.Create(&matrix).Error; err != nil {
				return err
			}

		case api.CoachingSessionReady:
			// Onboarding completed yesterday and nothing since, so the next deep session
			// (Movement) is unlocked today — the Journey screen shows the daily focus card
			// ready to start, with the commitments onboarding left behind.
			if err := seedRealUserProfile(tx, &user, now, journeyAfterOnboarding); err != nil {
				return err
			}
			if err := seedOpeningPair(tx, &user, now.AddDate(0, 0, -1)); err != nil {
				return err
			}
			if err := seedAppOpens(tx, userID, now, 0, 1); err != nil {
				return err
			}
		}

		// Save updated user record. balance_seconds is owned by the balance ledger
		// and intentionally preserved by test-setup (like communication_sessions) —
		// the model marks it <-:false, so this save skips it on its own.
		if err := tx.Save(&user).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to set up test data", zap.Error(err), zap.String("user_id", userID), zap.String("sessionType", string(sessionType)))
		http.Error(w, fmt.Sprintf(`{"error": "Failed to set up test data: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Successfully configured user to session type %s", string(sessionType)),
	})
}

// resetUserForTestSetup returns the user record to a just-signed-up state: everything the
// coaching journey wrote is cleared and the state goes back to the intro. The three
// registration details are cleared too — the intro collects country, date of birth, and
// gender in conversation, so a persona that starts before the intro must not already have
// them (and clearing them is what makes that collection phase testable at all). Personas
// that sit AFTER the intro put them back via seedRealUserProfile.
func resetUserForTestSetup(user *models.User) {
	user.IdealLifeVision = nil
	user.FocusArea = nil
	user.TopValues = nil
	user.JourneyTheme = nil
	user.JourneyQuoteCategory = nil
	user.LatestSessionHandle = nil
	user.LatestSessionHandleAt = nil
	user.State = sptr(string(models.StateOnboardingIntro))
	user.Country = nil
	user.DateOfBirth = nil
	user.Gender = nil
	// The persona contract: the journey the gates see must be EXACTLY what this run
	// seeds. Real sessions from previous QA rounds are preserved for analytics (see the
	// NOTE at the deletion block) but must stop counting — without this stamp, a tester
	// who completed Movement and Values last week and then reset to a fresh persona saw
	// the roadmap propose Energy right after Vision, with the sessions in between
	// missing (QA). Seeded test-seed-* rows bypass the cutoff inside JourneySessions,
	// so backdated seed histories keep working.
	now := time.Now()
	user.JourneyResetAt = &now
}

// ResetAccount implements api.ServerInterface. It returns the calling admin's own account
// to the state it had the moment it was provisioned — no history, no progress, no balance,
// no connected channel — so QA can retest a signup without creating a new address.
//
// It acts on the caller and nobody else. An admin id is enough to reach this, which is
// exactly why it must not take a target user: the endpoint destroys data no user-facing
// path can, and the blast radius stays the account of whoever calls it.
func (s *Server) ResetAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
		return
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := resetAccountData(tx, userID); err != nil {
			return err
		}
		resetUserForFreshAccount(&user)
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		// The Save above cannot write balance_seconds (<-:false on the model), and
		// this is the one reset that genuinely must: the ledger was deleted in this
		// same transaction, so the column has to reach zero with it or the invariant
		// balance = SUM(ledger) breaks the moment the reset commits.
		return tx.Exec(`UPDATE users SET balance_seconds = 0 WHERE id = ?`, userID).Error
	})
	if err != nil {
		s.logger.Error("failed to reset account", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "Failed to reset account"}`, http.StatusInternalServerError)
		return
	}

	// The bucket cannot join the transaction, so the screenshots follow the rows that
	// referenced them — same ordering as every other erasure path.
	s.purgeFeedbackObjects(r.Context(), userID)

	s.logger.Info("account reset to a freshly created state", zap.String("user_id", userID))
	w.WriteHeader(http.StatusNoContent)
}

// resetAccountData deletes every row the account owns. It is deliberately a hard delete of
// everything rather than the redact-and-keep of DELETE /me/data: the point is an account
// that has never been used, and a redacted session row still counts as a session to the
// journey, the streak and the usage calendar.
//
// Two rules the rest of the codebase holds are broken here on purpose, and only here:
//
//   - The minutes ledger (balance_transactions, channel_usage_records) is never deleted
//     anywhere else — it is the accounting record, and erasing coaching data is not a
//     refund. A test account's ledger is not accounting, and leaving it behind would keep
//     the free-session allowance spent on an account that is supposed to be new.
//   - Integrations and push tokens survive a data reset, because revoking them silently
//     disconnects a real person's WhatsApp. Here they go, because "brand new" means
//     nothing is still routed to this account.
//
// user_app_opens goes too. It survives the user-facing reset as a retention record, but it
// is dates against a user id and nothing reads it — on an account meant to look new it is
// just a trail of a previous life.
func resetAccountData(tx *gorm.DB, userID string) error {
	// goals is a legacy orphaned table — purge via raw SQL if it is still around.
	if tx.Migrator().HasTable("goals") {
		if err := tx.Exec("DELETE FROM goals WHERE user_id = ?", userID).Error; err != nil {
			return err
		}
	}

	// The chat conversation and the report screenshots have their own helpers because
	// each knows something extra: queued follow-ups go with the messages, and the
	// attachment rows must precede the objects purged after the commit.
	if err := deleteChatScope(tx, userID); err != nil {
		return err
	}
	if err := deleteFeedbackScope(tx, userID); err != nil {
		return err
	}

	// channel_usage_records is a legacy orphaned table (its events lost their last
	// reader when /me/usage narrowed to charged replies; spend lives in
	// ai_usage_records) — purge via raw SQL if it exists, like goals.
	if tx.Migrator().HasTable("channel_usage_records") {
		if err := tx.Exec("DELETE FROM channel_usage_records WHERE user_id = ?", userID).Error; err != nil {
			return err
		}
	}

	for _, model := range []interface{}{
		&models.BalanceTransaction{},
		&models.AIUsageRecord{},
		&models.BehaviorCheckIn{},
		&models.BehaviorPlan{},
		&models.CommitmentCompletion{},
		&models.Commitment{},
		&models.CommunicationSession{},
		&models.DailyJourney{},
		&models.EisenhowerMatrixExercise{},
		&models.IdentityReflection{},
		&models.Integration{},
		&models.Notification{},
		&models.Recommendation{},
		&models.UserAppOpen{},
		&models.UserBadge{},
		&models.UserDevice{},
		&models.UserMemory{},
		&models.WheelOfLifeExercise{},
	} {
		if err := tx.Where("user_id = ?", userID).Delete(model).Error; err != nil {
			return err
		}
	}
	return nil
}

// resetUserForFreshAccount puts the profile row back to what regional.CreateLocalProfile
// writes at signup: the identity fields it is given, and column defaults for everything
// else. Keep it in step with that function — it is the definition of "new account", and
// this is the only other place that has to reproduce it.
//
// What survives is what says WHICH account this is rather than what it did: id, email,
// phone, name, region and data region, preferred language, the consent timestamps and
// created_at. Country, date of birth and gender do NOT survive, even though provisioning
// can carry them: the onboarding intro is what collects them in conversation, and leaving
// them in place is what makes an intro rerun skip its collection phase.
//
// Distinct from resetUserForTestSetup, which prepares a persona and stamps journey_reset_at
// to fence off the session history it deliberately leaves behind. Here there is no history
// to fence, so the cutoff goes back to nil — a reset account has never reset anything.
func resetUserForFreshAccount(user *models.User) {
	user.DateOfBirth = nil
	user.Gender = nil
	user.Country = nil
	user.CoachGender = nil
	user.CoachVoice = nil
	user.IdealLifeVision = nil
	user.IdealLifeVisionSetAt = nil
	user.FocusArea = nil
	user.JourneyTheme = nil
	user.JourneyQuoteCategory = nil
	user.LatestSessionHandle = nil
	user.LatestSessionHandleAt = nil
	user.LastOnlineAt = nil
	user.LastPlatform = nil
	user.JourneyResetAt = nil
	user.DeletedAt = nil
	// The column defaults, spelled out: a Save() writes the struct as it stands, so an
	// unset field would persist the old value rather than fall back to the default.
	//
	// State is the one place this deviates from the raw column default. A provisioned row
	// carries VISION_IDEAL_LIFE, and the app then starts the onboarding intro explicitly —
	// ChatSession.CurrentState coerces the pair back to ONBOARDING_INTRO, so either value
	// runs the intro correctly. ONBOARDING_INTRO is written anyway, because it is the state
	// a new customer is actually IN at the start, it is what the new_user persona writes
	// (so the two admin actions leave the same account), and it is the value everything
	// reading the row directly — the journey's onboarding routing, the growth quote
	// category, Start-vs-Resume — needs to see to treat this as a first run.
	user.State = sptr(string(models.StateOnboardingIntro))
	user.Theme = sptr("waterfall")
	user.ChatHistoryRetentionDays = 7
	// Zero balance is the new-account state, and it is only coherent because the ledger
	// that explains it was deleted alongside. The free allowance is read from the
	// artifacts the opening pair produces (balance.FreeSessionAvailable) and the reset
	// above clears both — profile details and ideal_life_vision_set_at — so the account
	// gets its opening sessions back rather than being locked out at zero.
	//
	// In-memory only: balance_seconds is <-:false on the model, so the Save that
	// persists this struct skips the column. The caller zeroes it with raw SQL in the
	// same transaction that deleted the ledger — the two must move together, or the
	// account keeps a balance its (now empty) ledger cannot explain.
	user.BalanceSeconds = 0
}

// sptr returns a pointer to s (fixture-building convenience).
func sptr(s string) *string { return &s }

// seedWheelData is the completed Wheel of Life every post-onboarding persona shares.
// Career (score 5) is the priority area, matching users.focus_area.
const seedWheelData = `[
	{"name": "Health", "currentScore": 6, "reasoning": "Doing okay but I need more sleep"},
	{"name": "Relations", "currentScore": 7, "reasoning": "Good friends around me but work keeps me busy"},
	{"name": "Career", "currentScore": 5, "reasoning": "Feeling stagnation, waiting for something to change"},
	{"name": "Finances", "currentScore": 8, "reasoning": "Saving consistently, feeling secure"},
	{"name": "Growth", "currentScore": 6, "reasoning": "Learning slowly but without direction"}
]`

// seedKeyInsight is the user's named insight from the seeded onboarding session; it
// appears both as the newest "insight" memory (the Journey screen surfaces it) and in
// the onboarding session's synthesis payload.
const seedKeyInsight = "I realized my career stagnation comes from waiting for permission instead of creating my own opportunities."

// Journey stages a persona can be seeded at, passed to seedRealUserProfile. Commitments
// accumulate per completed deep session, the way they do for a real user.
const (
	journeyAfterOnboarding = 1 // onboarding done: the first steps the user named
	journeyAfterMovement   = 2 // + Movement done: the week's first step and today's proof
)

// seedRealUserProfile fills in everything a real user has after completing onboarding:
// vision + focus area on the user row, a fully scored wheel, memories in every category
// the app's memories screen filters (including the insights the Journey screen surfaces),
// commitments, and the AI's end-of-session picks (quote category + growth theme).
//
// deepSessionsDone says how far into the journey the persona is (journeyAfterOnboarding /
// journeyAfterMovement): every session leaves commitments behind, so a persona that has
// done two sessions has strictly more than one that just finished onboarding. Session
// history itself is seeded separately per persona (seedCompletedSession) because the
// timings are what distinguish the scenarios.
func seedRealUserProfile(tx *gorm.DB, user *models.User, now time.Time, deepSessionsDone int) error {
	userID := user.ID

	user.State = sptr(string(models.StateCheckin))
	// The intro is behind this user, so the details it collects are already in place.
	user.Country = sptr("PT")
	user.Gender = sptr("male")
	if dob, err := time.Parse("2006-01-02", "1990-05-03"); err == nil {
		user.DateOfBirth = &dob
	}
	user.IdealLifeVision = sptr("I live a balanced, peaceful life working on meaningful products, with time for nature, my health, and the people I love.")
	// Set alongside the vision itself, never left behind: it is the timestamp — not the
	// text — that balance.FreeSessionAvailable and the journey's opening-pair routing
	// read, so a persona missing it would be seeded as a fully onboarded user who is
	// somehow still owed free introductory sessions.
	user.IdealLifeVisionSetAt = &now
	user.FocusArea = sptr("Career")
	user.JourneyQuoteCategory = sptr("growth")
	user.JourneyTheme = sptr("sunset_beach")

	if err := tx.Create(&models.WheelOfLifeExercise{
		UserID:    userID,
		SessionID: "test-seed-onboarding-" + userID,
		Data:      seedWheelData,
	}).Error; err != nil {
		return err
	}

	// One memory per category save_memory can actually write — the api.MemoryCategory
	// enum and nothing else. A category outside it (e.g. "wheel_of_life") renders as a
	// bogus type in the app; the wheel lives in wheel_of_life_exercises, not in memories.
	// Insights come last so the Journey screen's "latest insight" picks the onboarding
	// one; staggered CreatedAt keeps the list ordering deterministic (newest first).
	memories := []models.UserMemory{
		{Category: string(api.Identity), Content: "Product designer at a Lisbon startup; grew up in Porto; father of a 3-year-old."},
		{Category: string(api.Values), Content: "Values autonomy, creativity, and deep connections with the few people who matter."},
		{Category: string(api.Needs), Content: "Needs structure and a calm morning routine to feel grounded."},
		{Category: string(api.Context), Content: "Currently juggling a product launch at work with very little personal time."},
		{Category: string(api.Obstacles), Content: "Tends to say yes to everything and postpones his own priorities, and Career and Finance are the areas he keeps deprioritizing."},
		{Category: string(api.Insight), Content: "Noticed that the weeks planned on Sunday evening are the ones with real progress."},
		{Category: string(api.Insight), Content: seedKeyInsight},
	}
	for i := range memories {
		memories[i].UserID = userID
		memories[i].CreatedAt = now.Add(-time.Duration(len(memories)-i) * time.Minute)
		if err := tx.Create(&memories[i]).Error; err != nil {
			return err
		}
	}

	// Commitments. Onboarding already leaves the user with the first steps they named
	// out loud (the CV move in the seeded transcript, plus a daily anchor), so even the
	// earliest post-onboarding persona has both render shapes — recurring and one-time.
	cvStep := now.AddDate(0, 0, 3).Format("2006-01-02")
	commitments := []models.Commitment{
		{ID: "seed-commit-1-" + userID, Origin: models.CommitmentOriginPlan, Title: "Update the CV and send it to two openings", Type: "one_time", Date: &cvStep},
		{ID: "seed-commit-2-" + userID, Origin: models.CommitmentOriginPlan, Title: "10-minute morning walk before opening the laptop", Type: "recurring", Days: models.IntSlice{1, 2, 3, 4, 5}},
	}

	// Movement adds the commitments registered on its interactive board — the week's
	// first step and today's proof — plus a manual one the user added themselves and
	// let slip, so the overdue state is exercised too.
	if deepSessionsDone >= journeyAfterMovement {
		proof := now.AddDate(0, 0, 1).Format("2006-01-02")
		slipped := now.AddDate(0, 0, -2).Format("2006-01-02")
		commitments = append(commitments,
			models.Commitment{ID: "seed-commit-3-" + userID, Origin: models.CommitmentOriginPlan, Title: "Write down one career opportunity to explore", Type: "recurring", Days: models.IntSlice{1, 3, 5}},
			models.Commitment{ID: "seed-commit-4-" + userID, Origin: models.CommitmentOriginPlan, Title: "Block 30 minutes to review the week", Type: "one_time", Date: &proof},
			models.Commitment{ID: "seed-commit-5-" + userID, Origin: models.CommitmentOriginManual, Title: "Call the gym to reactivate the membership", Type: "one_time", Date: &slipped},
		)
	}

	for i := range commitments {
		commitments[i].UserID = userID
		if err := tx.Create(&commitments[i]).Error; err != nil {
			return err
		}
	}

	return nil
}

// seedCompletedSession inserts a finished, typed communication session. IDs carry the
// "test-seed-" prefix so a rerun of test-setup replaces them without touching real
// session history. The onboarding session gets the full end-of-session artifacts a
// real one leaves behind — transcript, AI notes, evaluations, the user's spoken
// insight, and the persisted synthesis-screen payload (session_summary) the app
// re-renders from history.
func seedCompletedSession(tx *gorm.DB, user *models.User, sessionType api.SessionType, id string, end time.Time) error {
	// The onboarding intro is a couple of minutes; every other session runs long.
	duration := 25 * time.Minute
	if sessionType == api.SessionTypeOnboarding {
		duration = 3 * time.Minute
	}
	start := end.Add(-duration)
	sess := models.CommunicationSession{
		ID:           id,
		UserID:       user.ID,
		StartTime:    start,
		EndTime:      &end,
		Duration:     int(end.Sub(start).Seconds()),
		Language:     sptr("en-US"),
		SessionType:  sptr(string(sessionType)),
		AIEvaluation: func(f float64) *float64 { return &f }(4.6),
	}

	if sessionType == api.SessionTypeSessionVision {
		sess.UserEvaluation = func(f float64) *float64 { return &f }(5)
		sess.UserFeedback = sptr("Loved it — it felt like talking to someone who actually listens.")
		sess.UserSessionInsight = sptr(seedKeyInsight)
		sess.AINotes = sptr("User engaged deeply with the vision exercise. Career stagnation is the core theme; energy rose noticeably when talking about creating opportunities. Follow up on the morning-walk commitment.")
		sess.Transcript = sptr("[AI] Welcome. Before we begin, take a breath. What would your life look like three years from now if nothing held you back?\n[USER] Honestly... I picture myself leading my own product, more time outdoors, present with my family.\n[AI] What is the first small step this week toward that picture?\n[USER] I could finally update my CV and start looking at what is out there.")

		summary := map[string]interface{}{
			"session_id":   id,
			"session_type": string(api.SessionTypeSessionVision),
			"generated_at": end.UTC().Format(time.RFC3339),
			"next_session": map[string]string{"session_type": string(api.SessionTypeSessionMovement), "question_key": "next_question_movement_blockers"},
			"vision":       *user.IdealLifeVision,
			"priority_area": map[string]interface{}{
				"name":      "Career",
				"score":     5,
				"max_score": 10,
				"reasoning": "Feeling stagnation, waiting for something to change",
			},
			"key_insight": seedKeyInsight,
		}
		if b, err := json.Marshal(summary); err == nil {
			sess.SessionSummary = sptr(string(b))
		}
	}

	return tx.Create(&sess).Error
}

// seedOpeningPair seeds the two sessions every post-onboarding user has behind them:
// the short onboarding intro and the Vision session it hands over to. endVision is when
// Vision finished; the intro is placed shortly before it. Personas must seed BOTH — a
// user with only the intro is still mid-journey and would be proposed Vision, not the
// scenario the persona is named for.
func seedOpeningPair(tx *gorm.DB, user *models.User, endVision time.Time) error {
	if err := seedCompletedSession(tx, user, api.SessionTypeOnboarding, "test-seed-onboarding-"+user.ID, endVision.Add(-30*time.Minute)); err != nil {
		return err
	}
	return seedCompletedSession(tx, user, api.SessionTypeSessionVision, "test-seed-vision-"+user.ID, endVision)
}

// seedAppOpens records app-open rows (streak source) for each of the given day
// offsets (0 = today, 1 = yesterday, ...).
func seedAppOpens(tx *gorm.DB, userID string, now time.Time, daysAgo ...int) error {
	for _, d := range daysAgo {
		day := now.AddDate(0, 0, -d)
		open := models.UserAppOpen{
			UserID:   userID,
			OpenDate: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		}
		if err := tx.Create(&open).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetTestDataDocs implements api.ServerInterface
func (s *Server) GetTestDataDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(testDataDocsHTML))
}

const testDataDocsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>QA Test Data Setup Portal</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0f172a;
            --card-bg: rgba(30, 41, 59, 0.7);
            --border-color: rgba(255, 255, 255, 0.08);
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent-primary: #6366f1;
            --accent-secondary: #a855f7;
            --success-color: #10b981;
        }
        
        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }
        
        body {
            font-family: 'Outfit', sans-serif;
            background-color: var(--bg-color);
            background-image: 
                radial-gradient(at 0% 0%, rgba(99, 102, 241, 0.15) 0px, transparent 50%),
                radial-gradient(at 100% 100%, rgba(168, 85, 247, 0.15) 0px, transparent 50%);
            background-attachment: fixed;
            color: var(--text-primary);
            line-height: 1.6;
            padding: 2rem 1.5rem;
        }

        .container {
            max-width: 1100px;
            margin: 0 auto;
        }

        header {
            text-align: center;
            margin-bottom: 3.5rem;
            animation: fadeInDown 0.8s ease-out;
        }

        h1 {
            font-size: 2.5rem;
            font-weight: 700;
            margin-bottom: 0.75rem;
            background: linear-gradient(135deg, #818cf8 0%, #c084fc 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .subtitle {
            color: var(--text-secondary);
            font-size: 1.1rem;
            max-width: 600px;
            margin: 0 auto;
        }

        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
            gap: 1.5rem;
            animation: fadeInUp 1s ease-out;
        }

        .db-btn {
            background: rgba(99, 102, 241, 0.1);
            border: 1px solid rgba(99, 102, 241, 0.2);
            color: #a5b4fc;
            padding: 0.4rem 0.8rem;
            border-radius: 8px;
            cursor: pointer;
            font-size: 0.8rem;
            transition: all 0.2s;
            font-family: inherit;
        }

        .db-btn:hover {
            background: rgba(99, 102, 241, 0.25);
            border-color: rgba(99, 102, 241, 0.4);
            color: white;
        }

        .db-btn.active {
            background: rgba(168, 85, 247, 0.15);
            border-color: rgba(168, 85, 247, 0.3);
            color: #d8b4fe;
        }

        .db-data-container {
            margin-top: 1rem;
            border-top: 1px dashed rgba(255, 255, 255, 0.08);
            padding-top: 0.75rem;
            text-align: left;
        }

        .db-data-container pre {
            background: #0b0f19;
            border: 1px solid rgba(255, 255, 255, 0.04);
            border-radius: 12px;
            padding: 0.8rem;
            font-family: 'Courier New', Courier, monospace;
            font-size: 0.8rem;
            color: #a5b4fc;
            overflow-x: auto;
            max-height: 200px;
        }

        .card {
            background: var(--card-bg);
            backdrop-filter: blur(12px);
            border: 1px solid var(--border-color);
            border-radius: 20px;
            padding: 1.5rem;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            display: flex;
            flex-direction: column;
            justify-content: space-between;
        }

        .card:hover {
            transform: translateY(-6px);
            border-color: rgba(255, 255, 255, 0.2);
            box-shadow: 0 15px 30px rgba(0, 0, 0, 0.3);
        }

        .badge {
            display: inline-block;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            padding: 0.25rem 0.6rem;
            border-radius: 6px;
            margin-bottom: 1rem;
        }

        .badge-onboarding {
            background: rgba(99, 102, 241, 0.15);
            color: #818cf8;
            border: 1px solid rgba(99, 102, 241, 0.3);
        }

        .badge-checkin {
            background: rgba(16, 185, 129, 0.15);
            color: #34d399;
            border: 1px solid rgba(16, 185, 129, 0.3);
        }

        .badge-stress {
            background: rgba(239, 68, 68, 0.15);
            color: #f87171;
            border: 1px solid rgba(239, 68, 68, 0.3);
        }

        .card-title {
            font-size: 1.15rem;
            font-weight: 600;
            margin-bottom: 0.5rem;
            color: white;
            font-family: monospace;
            cursor: pointer;
        }

        .card-state {
            font-size: 0.85rem;
            color: var(--text-secondary);
            margin-bottom: 1rem;
            font-style: italic;
        }

        .card-details {
            font-size: 0.9rem;
            color: #cbd5e1;
            margin-bottom: 1.5rem;
        }

        .card-commitment {
            display: flex;
            justify-content: flex-end;
            margin-top: auto;
        }

        @keyframes fadeInDown {
            from {
                opacity: 0;
                transform: translateY(-20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        @keyframes fadeInUp {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>QA Test Data Setup Portal</h1>
            <p class="subtitle">Quick reference for the <code>POST /v1/admin/test-setup</code> personas. The onboarding personas seed a partially-completed onboarding; every persona after it seeds a <strong>fully furnished real user</strong> — vision, focus area, scored wheel, memories and insights, commitments of every shape, AI theme/quote picks, and typed session history with transcripts and the synthesis screen. Real session history is preserved; only <code>test-seed-*</code> sessions from a previous run are replaced, so reruns are idempotent.</p>
        </header>

        <div class="grid">
            <!-- Cards of session types will be rendered here dynamically -->
        </div>
    </div>

    <script>
        // Every post-onboarding persona seeds this full real-user profile, on top of its
        // scenario-specific session history. Mirrors seedRealUserProfile in admin.go.
        const PROFILE_BASE = {
            user: {
                state: "checkin",
                ideal_life_vision: "I live a balanced, peaceful life working on meaningful products, with time for nature, my health, and the people I love.",
                focus_area: "Career",
                journey_quote_category: "growth",
                journey_theme: "sunset_beach"
            },
            wheel_of_life: [
                { name: "Health", currentScore: 6 }, { name: "Relations", currentScore: 7 },
                { name: "Career", currentScore: 5, priority: true }, { name: "Finances", currentScore: 8 },
                { name: "Growth", currentScore: 6 }
            ],
            user_memories: "8 memories across identity / values / needs / context / obstacles / wheel_of_life + 2 insights (newest = the onboarding key insight the Journey screen surfaces)",
            onboarding_session: "typed communication session (25 min) with transcript, AI notes, evaluations, user feedback, key insight and the persisted session_summary synthesis payload"
        };

        // Commitments accumulate per completed session, the way they do for a real user:
        // onboarding leaves the first steps the user named out loud, Movement adds the
        // ones registered on its board plus a manual one that slipped (overdue).
        const COMMITMENTS_AFTER_ONBOARDING = [
            { id: "seed-commit-1", origin: "plan", title: "Update the CV and send it to two openings", type: "one_time", date: "now + 3d" },
            { id: "seed-commit-2", origin: "plan", title: "10-minute morning walk before opening the laptop", type: "recurring", days: [1, 2, 3, 4, 5] }
        ];
        const COMMITMENTS_AFTER_MOVEMENT = COMMITMENTS_AFTER_ONBOARDING.concat([
            { id: "seed-commit-3", origin: "plan", title: "Write down one career opportunity to explore", type: "recurring", days: [1, 3, 5] },
            { id: "seed-commit-4", origin: "plan", title: "Block 30 minutes to review the week", type: "one_time", date: "now + 1d" },
            { id: "seed-commit-5", origin: "manual", title: "Call the gym to reactivate the membership", type: "one_time", date: "now - 2d (overdue)" }
        ]);

        // FULL_PROFILE(commitments) — the profile as seeded at a given journey stage.
        const FULL_PROFILE = (commitments) => Object.assign({}, PROFILE_BASE, { commitments: commitments });

        const sessionTypes = [
            {
                name: "new_user",
                badge: "onboarding",
                state: "State: onboarding",
                details: "Resets the user's database records completely. Clears commitments, memories and exercises; the user starts at the onboarding introduction stage. Real session history is preserved — only test-seed-* rows are replaced — so the account keeps its analytics. For an account with no history, no balance and no connected channel at all, use the app's admin \"Reset account\" action (POST /admin/reset-account).",
                dbData: {
                    user: { state: "onboarding", ideal_life_vision: null, tasks: null },
                    deleted_records: ["goals", "user_memories", "eisenhower_matrix_exercises", "wheel_of_life_exercises", "daily_journeys", "communication_sessions", "user_app_opens"]
                }
            },
            {
                name: "onboarding_ideal_vision",
                badge: "onboarding",
                state: "State: ideal_life_vision",
                details: "Sets user state to the Ideal Life Vision questionnaire phase, ready to receive and record user's 3-year ideal vision.",
                dbData: {
                    user: { state: "ideal_life_vision", ideal_life_vision: null, tasks: null },
                    deleted_records: ["goals", "user_memories", "eisenhower_matrix_exercises", "wheel_of_life_exercises", "daily_journeys", "communication_sessions", "user_app_opens"]
                }
            },
            {
                name: "onboarding_wheel",
                badge: "onboarding",
                state: "State: wheel_of_life",
                details: "Pre-fills user Ideal Life Vision and starts the Wheel of Life category scoring exercise. A default vision is seeded.",
                dbData: {
                    user: { state: "wheel_of_life", ideal_life_vision: "I live a balanced, peaceful life working on meaningful products with time for nature and health." },
                    wheel_of_life_exercise: [
                        { name: "Health", currentScore: 6, reasoning: "Doing okay but could sleep more" },
                        { name: "Relations", currentScore: 0, reasoning: "" },
                        { name: "Career", currentScore: 0, reasoning: "" },
                        { name: "Finances", currentScore: 0, reasoning: "" },
                        { name: "Growth", currentScore: 0, reasoning: "" }
                    ]
                }
            },
            {
                name: "onboarding_metaphor",
                badge: "onboarding",
                state: "State: metaphor",
                details: "Sets up a completely scored Wheel of Life (Health: 6, Relations: 7, Career: 5, Finances: 8, Growth: 6) to test the Transformational Metaphor step.",
                dbData: {
                    user: { state: "metaphor", ideal_life_vision: "I live a balanced, peaceful life working on meaningful products with time for nature and health." },
                    wheel_of_life_exercise: [
                        { name: "Health", currentScore: 6, reasoning: "Need more sleep" },
                        { name: "Relations", currentScore: 7, reasoning: "Good friends but busy" },
                        { name: "Career", currentScore: 5, reasoning: "Feeling stagnation" },
                        { name: "Finances", currentScore: 8, reasoning: "Saving well" },
                        { name: "Growth", currentScore: 6, reasoning: "Learning slowly" }
                    ]
                }
            },
            {
                name: "onboarding_ending",
                badge: "onboarding",
                state: "State: onboarding_emotional_closing",
                details: "Configures a fully scored Wheel of Life and an established Goal. AI will guide user to emotional closing and then end the session.",
                dbData: {
                    user: { state: "onboarding_emotional_closing", ideal_life_vision: "I live a balanced, peaceful life working on meaningful products with time for nature and health." },
                    wheel_of_life_exercise: [
                        { name: "Health", currentScore: 6, reasoning: "Need more sleep" },
                        { name: "Relations", currentScore: 7, reasoning: "Good friends but busy" },
                        { name: "Career", currentScore: 5, reasoning: "Feeling stagnation" },
                        { name: "Finances", currentScore: 8, reasoning: "Saving well" },
                        { name: "Growth", currentScore: 6, reasoning: "Learning slowly" }
                    ],
                    goal: {
                        area: "Career",
                        goal: "Transition to a product management role in 6 months"
                    }
                }
            },
            {
                name: "checkin_daily_first",
                badge: "checkin",
                state: "State: checkin — first day after onboarding",
                details: "Full real-user profile with onboarding finished 2 hours ago: the very first check-in day. Movement unlocks tomorrow, so the Journey screen offers today's check-in plus the next-session preview.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_ONBOARDING),
                    communication_sessions: [{ id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 2h" }],
                    user_app_opens: ["today"]
                }
            },
            {
                name: "checkin_daily_same_day",
                badge: "checkin",
                state: "State: checkin — already checked in today",
                details: "Full real-user profile that already did today's check-in 2 hours ago (onboarding 10d ago, movement 2d ago). Tests double check-in and daily lock validation.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 10d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 2d" },
                        { id: "test-seed-checkin-1", type: "checkin", end_time: "Now - 2h" }
                    ],
                    user_app_opens: ["today", "1 day ago", "2 days ago"]
                }
            },
            {
                name: "checkin_daily_consecutive",
                badge: "checkin",
                state: "State: checkin — 3-day streak",
                details: "Full real-user profile with check-ins on each of the last 3 days (onboarding 10d ago, movement 4d ago — the next deep session is still gated). Tests streak validation and incrementing.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 10d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 4d" },
                        { id: "test-seed-checkin-1..3", type: "checkin", end_time: "1 / 2 / 3 days ago" }
                    ],
                    user_app_opens: ["1 day ago", "2 days ago", "3 days ago"]
                }
            },
            {
                name: "checkin_daily_after_days",
                badge: "checkin",
                state: "State: checkin — 4-day gap",
                details: "Full real-user profile whose last activity (movement session) was 4 days ago. Tests the 4-day-gap opening prompts; the next deep session unlocks tomorrow.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 14d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 4d" }
                    ],
                    user_app_opens: ["4 days ago"]
                }
            },
            {
                name: "checkin_daily_after_weeks",
                badge: "checkin",
                state: "State: checkin — 12-day gap",
                details: "Full real-user profile away for 12 days (onboarding 22d ago, movement 12d ago). The next deep session (values) is overdue, exactly like a real user returning after two weeks — the check-in opens with the gap acknowledgement and can then offer to switch into it.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 22d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 12d" }
                    ],
                    user_app_opens: ["12 days ago"]
                }
            },
            {
                name: "checkin_daily_after_long",
                badge: "checkin",
                state: "State: checkin — 35-day gap",
                details: "Full real-user profile away for 35 days (onboarding 45d ago, movement 35d ago). Tests the >1 month re-engagement flow. Like the 12-day persona, the next deep session is long overdue, so the check-in does its gap opening first and then offers to switch — exactly what a returning user gets.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 45d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 35d" }
                    ],
                    user_app_opens: ["35 days ago"]
                }
            },
            {
                name: "stressed",
                badge: "stress",
                state: "State: checkin + overdue pile + Matrix",
                details: "Full real-user profile plus 4 overdue one-time commitments (dates relative to today) and a populated Eisenhower Matrix (Chaos Sorter), to test stress-mitigation coaching.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_MOVEMENT),
                    communication_sessions: [
                        { id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 10d" },
                        { id: "test-seed-movement", type: "session_movement", end_time: "Now - 2d" }
                    ],
                    extra_overdue_tasks: [
                        { id: "task-1", title: "Overdue important medical appointment", type: "one_time", date: "Now - 15d" },
                        { id: "task-2", title: "Prepare presentation slide deck", type: "one_time", date: "Now - 10d" },
                        { id: "task-3", title: "Review budget options", type: "one_time", date: "Now - 6d" },
                        { id: "task-4", title: "Organize filing cabinets", type: "one_time", date: "Now - 3d" }
                    ],
                    eisenhower_matrix_exercise: {
                        urgent_important: [
                            { task: "Overdue important medical appointment", reasoning: "Crucial health checkup" },
                            { task: "Prepare presentation slide deck", reasoning: "Presentation due tomorrow" }
                        ],
                        not_urgent_important: [],
                        urgent_not_important: [
                            { task: "Review budget options", reasoning: "Interruption from colleague" }
                        ],
                        not_urgent_not_important: [
                            { task: "Organize filing cabinets", reasoning: "Low priority task" }
                        ]
                    },
                    user_app_opens: ["today", "1 day ago", "2 days ago"]
                }
            },
            {
                name: "coaching_session_ready",
                badge: "checkin",
                state: "State: checkin — deep session unlocked",
                details: "Full real-user profile with onboarding completed yesterday: the Movement session is unlocked today, so the Journey screen shows the daily focus card ready to start.",
                dbData: {
                    full_profile: FULL_PROFILE(COMMITMENTS_AFTER_ONBOARDING),
                    communication_sessions: [{ id: "test-seed-onboarding", type: "onboarding", end_time: "Now - 1d" }],
                    user_app_opens: ["today", "1 day ago"]
                }
            }
        ];

        const grid = document.querySelector('.grid');
        sessionTypes.forEach(st => {
            const card = document.createElement('div');
            card.className = 'card';
            
            const badgeClass = 'badge-' + st.badge;
            const payload = JSON.stringify({ sessionType: st.name }, null, 2);
            
            card.innerHTML = '<div>' +
                '<span class="badge ' + badgeClass + '">' + st.badge + '</span>' +
                '<h3 class="card-title">' + st.name + '</h3>' +
                '<div class="card-state">' + st.state + '</div>' +
                '<div class="card-details">' + st.details + '</div>' +
                '</div>' +
                '<div class="db-data-container" style="display: none;">' +
                '<pre><code>' + JSON.stringify(st.dbData, null, 2) + '</code></pre>' +
                '</div>' +
                '<div class="card-commitment">' +
                '<button class="db-btn" onclick="toggleDbData(this)">View Seeded DB Data</button>' +
                '</div>';
            grid.appendChild(card);
        });

        function toggleDbData(btn) {
            const container = btn.closest('.card').querySelector('.db-data-container');
            if (container.style.display === 'none') {
                container.style.display = 'block';
                btn.innerText = 'Hide DB Data';
                btn.classList.add('active');
            } else {
                container.style.display = 'none';
                btn.innerText = 'View Seeded DB Data';
                btn.classList.remove('active');
            }
        }
    </script>
</body>
</html>
`
