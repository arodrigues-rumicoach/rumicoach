package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/badge"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// GetMeProfile implements api.ServerInterface.
func (s *Server) GetMeProfile(w http.ResponseWriter, r *http.Request) {
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

	currentStreak, longestStreak, _, _, err := s.CalculateUserStreak(userID, GetTimezoneLocation(r))
	if err != nil {
		s.logger.Error("failed to calculate streak", zap.Error(err))
		http.Error(w, `{"error": "Failed to calculate streak"}`, http.StatusInternalServerError)
		return
	}

	// 1. Life Balance
	var latestWheel models.WheelOfLifeExercise
	var wheelCompletedAt *time.Time
	var wheelData *string

	if err := database.DB.Where("user_id = ?", userID).Order("created_at desc").First(&latestWheel).Error; err == nil {
		wheelCompletedAt = &latestWheel.UpdatedAt
		wheelData = &latestWheel.Data
	}

	// 2. Total Sessions & Hours with Rumi
	var sessions []models.CommunicationSession
	database.DB.Where("user_id = ?", userID).Find(&sessions)
	totalSessions := len(sessions)
	var totalSeconds float64
	for _, sess := range sessions {
		if sess.EndTime != nil {
			totalSeconds += float64(sess.Duration)
		}
	}
	hoursWithRumi := float32(totalSeconds / 3600.0)

	// 3. Commitments Kept
	var oneTimeKept int64
	database.DB.Model(&models.Commitment{}).Where("user_id = ? AND done = ?", userID, true).Count(&oneTimeKept)

	var behaviorKept int64
	database.DB.Model(&models.BehaviorCheckIn{}).Where("user_id = ? AND status = ?", userID, models.BehaviorCheckInKept).Count(&behaviorKept)

	commitmentsKept := int(oneTimeKept + behaviorKept)

	// 4. Insights Discovered
	var insightsDiscovered int64
	database.DB.Model(&models.UserMemory{}).Where("user_id = ? AND category = ?", userID, "insight").Count(&insightsDiscovered)

	// 5. Evaluate Badges. The badge service also runs at session end, so most badges
	// are already persisted the moment they're achieved; this call catches the ones
	// whose conditions changed outside a session (an activated integration, a
	// commitment marked done from the Journey screen).
	badge.EvaluateAndAward(userID, GetTimezoneLocation(r), s.logger)

	var allBadges []models.UserBadge
	database.DB.Where("user_id = ?", userID).Order("earned_at asc").Find(&allBadges)

	// Map to API response. The retired goalReached type may still exist as rows —
	// kept for analytics; the app ignores types it doesn't recognize.
	var apiBadges []api.ProfileBadge
	for _, b := range allBadges {
		badgeType := api.BadgeType(b.BadgeType)
		apiBadges = append(apiBadges, api.ProfileBadge{
			Type:     &badgeType,
			EarnedAt: &b.EarnedAt,
		})
	}

	progress := api.ProfileProgress{
		BestStreak:         &longestStreak,
		CommitmentsKept:    &commitmentsKept,
		CurrentStreak:      &currentStreak,
		HoursWithRumi:      &hoursWithRumi,
		InsightsDiscovered: func() *int { v := int(insightsDiscovered); return &v }(),
		TotalSessions:      &totalSessions,
	}

	var completedAtDate *openapi_types.Date
	if wheelCompletedAt != nil {
		completedAtDate = &openapi_types.Date{Time: *wheelCompletedAt}
	}

	lifeBalance := api.ProfileBalance{
		CompletedAt: completedAtDate,
		Data:        wheelData,
	}

	resp := api.UserProfileResponse{
		FocusArea:   user.FocusArea,
		Progress:    &progress,
		Badges:      &apiBadges,
		LifeBalance: &lifeBalance,
	}

	if user.IdealLifeVision != nil {
		resp.Vision = &api.ProfileVision{
			Text:      user.IdealLifeVision,
			CraftedAt: user.IdealLifeVisionSetAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
