package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/companion"
	"github.com/rumi/rumi-be/internal/services/regional"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Server implements api.ServerInterface
type Server struct {
	logger          *zap.Logger
	regional        *regional.Client
	whatsappWebhook *WhatsAppWebhookHandler
	telegramWebhook *TelegramWebhookHandler
	twilioWebhook   *TwilioWebhookHandler
}

func NewServer(logger *zap.Logger, whatsappWebhook *WhatsAppWebhookHandler, telegramWebhook *TelegramWebhookHandler, twilioWebhook *TwilioWebhookHandler) *Server {
	return &Server{
		logger:          logger,
		regional:        regional.NewClient(),
		whatsappWebhook: whatsappWebhook,
		telegramWebhook: telegramWebhook,
		twilioWebhook:   twilioWebhook,
	}
}

// GetCurrentUser implements api.ServerInterface
func (s *Server) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
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

	loc := GetTimezoneLocation(r)
	platform := ExtractPlatform(r)
	// Track today as an app-open day and update online status in the background — idempotent, no latency impact.
	go func(loc *time.Location, plt string) {
		nowLocal := time.Now().In(loc)
		today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)
		record := models.UserAppOpen{UserID: userID, OpenDate: today}
		// Parallel /me requests race here; the unique index resolves the tie, so a conflict is not an error.
		database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "open_date"}},
			DoNothing: true,
		}).Create(&record) //nolint:errcheck

		now := time.Now()
		userUpdates := map[string]interface{}{
			"last_online_at": now,
		}
		if plt != "" {
			userUpdates["last_platform"] = plt
		}
		database.DB.Model(&models.User{}).Where("id = ?", userID).Updates(userUpdates)

		identityUpdates := map[string]interface{}{
			"last_online_at": now,
		}
		if plt != "" {
			identityUpdates["platform"] = plt
		}
		database.Auth().Model(&models.Identity{}).Where("id = ?", userID).Updates(identityUpdates)
	}(loc, platform)

	var dob *openapi_types.Date
	if user.DateOfBirth != nil {
		dob = &openapi_types.Date{Time: *user.DateOfBirth}
	}

	theme := "waterfall"
	if user.Theme != nil && *user.Theme != "" {
		theme = *user.Theme
	}

	var coachVoice *api.UserCoachVoice
	if user.CoachVoice != nil {
		cv := api.UserCoachVoice(*user.CoachVoice)
		coachVoice = &cv
	}

	// Reported for display and for app builds already in the field, which mirror the
	// WebSocket's rule off this field to pre-empt the call. Current builds do not: the
	// socket now says no in a way a client can read, so this is no longer load-bearing
	// and nothing may treat it as the enforcement.
	//
	// The field keeps its name because those shipped builds read it; what changed is
	// how it is decided — whether the introductory sessions have produced their
	// artifacts, not a row count and not a position in users.state. An error reports
	// false (not free), matching the direction the session itself takes.
	inFirstJourney, err := balance.OpeningPairUnfinished(ctx, userID)
	if err != nil {
		s.logger.Error("Free-session check failed for profile", zap.Error(err), zap.String("user_id", userID))
	}

	apiUser := api.User{
		Id:                           &user.ID,
		Email:                        user.Email,
		PhoneNumber:                  user.PhoneNumber,
		Name:                         user.Name,
		DateOfBirth:                  dob,
		Gender:                       user.Gender,
		CoachGender:                  user.CoachGender,
		CoachVoice:                   coachVoice,
		Country:                      user.Country,
		Region:                       user.Region,
		PreferredLanguage:            user.PreferredLanguage,
		IsActive:                     &user.IsActive,
		Theme:                        &theme,
		FocusArea:                    user.FocusArea,
		TermsAndConditionsAcceptedAt: user.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          user.MarketingAcceptedAt,
		AiAcceptedAt:                 user.AIAcceptedAt,
		LastOnlineAt:                 user.LastOnlineAt,
		LastPlatform:                 user.LastPlatform,
		BalanceSeconds:               &user.BalanceSeconds,
		InFirstJourney:               &inFirstJourney,
		ChatHistoryRetentionDays:     &user.ChatHistoryRetentionDays,
		CreatedAt:                    &user.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUser)
}

// UpdateCurrentUser implements api.ServerInterface
func (s *Server) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
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

	var req api.UpdateCurrentUserJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid update format"}`, http.StatusBadRequest)
		return
	}

	if req.DateOfBirth != nil {
		user.DateOfBirth = &req.DateOfBirth.Time
	}
	if req.Gender != nil {
		user.Gender = req.Gender
	}
	if req.CoachGender != nil {
		user.CoachGender = req.CoachGender
	}
	if req.Country != nil {
		user.Country = req.Country
	}
	if req.Region != nil {
		user.Region = req.Region
	}
	if req.PreferredLanguage != nil {
		user.PreferredLanguage = req.PreferredLanguage
	}
	if req.Theme != nil {
		user.Theme = req.Theme
		// A manual theme choice overrides the AI's end-of-session Journey-screen pick:
		// clearing it makes /journey fall back to users.theme until the next
		// session picks a new one (most recent choice wins, whoever made it).
		user.JourneyTheme = nil
	}
	if req.Name != nil {
		user.Name = req.Name
	}
	if req.CoachVoice != nil {
		cv := string(*req.CoachVoice)
		user.CoachVoice = &cv
	}

	// The three consents, for the users registration never asked. "Continue with Apple" (or
	// Google) on the sign-in screen creates an account without going through the signup
	// steps, so that user holds none of them — not even the Terms — and neither do accounts
	// older than a given consent. The app asks before their first session and PATCHes the
	// answers here.
	//
	// Stamped from the server's clock rather than taken from the client, and only when not
	// already set: when consent was first given is the auditable fact, so a repeat must not
	// move it. `false` is ignored rather than treated as a withdrawal — revoking a required
	// consent means deleting the account, not editing a profile field, and clearing a stamp
	// here would bounce the user back to a gate they just answered.
	now := time.Now()
	if req.TermsAndConditionsAccepted != nil && *req.TermsAndConditionsAccepted && user.TermsAndConditionsAcceptedAt == nil {
		user.TermsAndConditionsAcceptedAt = &now
	}
	if req.AiAccepted != nil && *req.AiAccepted && user.AIAcceptedAt == nil {
		user.AIAcceptedAt = &now
	}
	if req.MarketingAccepted != nil && *req.MarketingAccepted && user.MarketingAcceptedAt == nil {
		user.MarketingAcceptedAt = &now
	}

	// Chat retention is applied through the companion service, not written here: the value
	// is materialized onto every stored message, and changing it has to rewrite those in
	// the same transaction or the existing conversation keeps living by the old rule. Done
	// BEFORE the row save below so a rejected value cannot half-apply.
	if req.ChatHistoryRetentionDays != nil {
		days := int(*req.ChatHistoryRetentionDays)
		if !models.ValidChatRetention(days) {
			apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload,
				"chatHistoryRetentionDays must be one of: 0, 3, 7, 30")
			return
		}
		if err := companion.SetChatHistoryRetention(database.DB, userID, days); err != nil {
			s.logger.Error("failed to set chat history retention",
				zap.String("user_id", userID), zap.Int("days", days), zap.Error(err))
			http.Error(w, `{"error": "Failed to update chat history retention"}`, http.StatusInternalServerError)
			return
		}
		user.ChatHistoryRetentionDays = days
	}

	// balance_seconds is owned by the balance ledger (internal/services/balance);
	// the model marks it <-:false, so this full-row save skips it and cannot
	// clobber a concurrent session debit.
	database.DB.Save(&user)

	var dob2 *openapi_types.Date
	if user.DateOfBirth != nil {
		dob2 = &openapi_types.Date{Time: *user.DateOfBirth}
	}

	theme2 := "waterfall"
	if user.Theme != nil && *user.Theme != "" {
		theme2 = *user.Theme
	}

	var coachVoice2 *api.UserCoachVoice
	if user.CoachVoice != nil {
		cv := api.UserCoachVoice(*user.CoachVoice)
		coachVoice2 = &cv
	}

	// Same rule and same failure direction as GetCurrentUser — see the note there.
	// Recomputed rather than carried over: this handler is what writes the profile
	// details, so a PATCH that completes them can be the very thing that ends the
	// introductory allowance.
	inFirstJourney2, freeErr := balance.OpeningPairUnfinished(ctx, userID)
	if freeErr != nil {
		s.logger.Error("Free-session check failed for profile", zap.Error(freeErr), zap.String("user_id", userID))
	}

	apiUser := api.User{
		Id:                           &user.ID,
		Email:                        user.Email,
		PhoneNumber:                  user.PhoneNumber,
		Name:                         user.Name,
		DateOfBirth:                  dob2,
		Gender:                       user.Gender,
		CoachGender:                  user.CoachGender,
		CoachVoice:                   coachVoice2,
		Country:                      user.Country,
		Region:                       user.Region,
		PreferredLanguage:            user.PreferredLanguage,
		IsActive:                     &user.IsActive,
		Theme:                        &theme2,
		FocusArea:                    user.FocusArea,
		TermsAndConditionsAcceptedAt: user.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          user.MarketingAcceptedAt,
		AiAcceptedAt:                 user.AIAcceptedAt,
		LastOnlineAt:                 user.LastOnlineAt,
		LastPlatform:                 user.LastPlatform,
		BalanceSeconds:               &user.BalanceSeconds,
		InFirstJourney:               &inFirstJourney2,
		ChatHistoryRetentionDays:     &user.ChatHistoryRetentionDays,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiUser)
}

// DeleteCurrentUserData implements api.ServerInterface.
//
// Without ?scope= it erases everything, which is what callers written before the
// parameter expected. With it, one or more categories are erased instead — all inside a
// single transaction, so a scope that cannot finish repairing its derived state (the
// badges it earned, above all) takes the whole request down with it rather than leaving
// the account half-consistent.
func (s *Server) DeleteCurrentUserData(w http.ResponseWriter, r *http.Request, params api.DeleteCurrentUserDataParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	raw := ""
	if params.Scope != nil {
		raw = *params.Scope
	}
	scopes, parseErr := parseDataScopes(raw)
	if parseErr != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, parseErr.Error())
		return
	}

	// The daily-growth snapshot is keyed by the user's own calendar date, so which row
	// counts as "today" depends on their timezone.
	loc := GetTimezoneLocation(r)

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, sc := range scopes {
			var scopeErr error
			switch sc {
			case scopeMemories:
				scopeErr = deleteMemoriesScope(tx, userID)
			case scopeChat:
				scopeErr = deleteChatScope(tx, userID)
			case scopeCommitments:
				scopeErr = deleteCommitmentsScope(tx, userID)
			case scopeProgress:
				scopeErr = deleteProgressScope(tx, userID, loc)
			case scopeAll:
				scopeErr = s.deleteAllUserData(tx, userID, loc)
			}
			if scopeErr != nil {
				return scopeErr
			}
		}
		return nil
	})

	if err != nil {
		s.logger.Error("failed to delete user data", zap.String("user_id", userID),
			zap.String("scope", raw), zap.Error(err))
		http.Error(w, `{"error": "Failed to delete user data"}`, http.StatusInternalServerError)
		return
	}

	// Only now the transaction has committed: the bucket cannot be rolled back with it.
	for _, sc := range scopes {
		if sc == scopeAll {
			s.purgeFeedbackObjects(r.Context(), userID)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteAllUserData is the `all` scope: every category at once, plus the profile fields
// that make the account read as a fresh start.
func (s *Server) deleteAllUserData(tx *gorm.DB, userID string, loc *time.Location) error {
	{
		updates := map[string]interface{}{
			"ideal_life_vision":        nil,
			"top_values":               nil,
			"ideal_life_vision_set_at": nil,
			"focus_area":               nil,
			"state":                    string(models.StateVisionIdealLife),
			"latest_session_handle":    nil,
			"latest_session_handle_at": nil,
		}

		// balance_seconds is deliberately absent from this map: the minutes the user paid
		// for are theirs, and erasing their coaching data is not a refund.
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return err
		}

		if err := deleteMemoriesScope(tx, userID); err != nil {
			return err
		}

		// The chat log is content, and until now it outlived every erasure path there is.
		if err := deleteChatScope(tx, userID); err != nil {
			return err
		}

		// Feedback carries the user's words, and its screenshots carry whatever was on
		// their screen — memories, vision, session text. The bucket cannot join this
		// transaction, so the rows go here and the objects follow once it commits.
		if err := deleteFeedbackScope(tx, userID); err != nil {
			return err
		}

		// communication_sessions rows are NOT deleted: alongside the transcript they carry
		// the measurements the business runs on — duration, per-modality token counts,
		// Deepgram seconds, STT service. Those are usage and cost analytics, not personal
		// content. So the row stays and everything about the user inside it is stripped,
		// the same line we drew between balance_transactions and the coaching tables.
		if err := redactSessionContent(tx, userID); err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.EisenhowerMatrixExercise{}).Error; err != nil {
			return err
		}

		// goals is a legacy orphaned table — purge via raw SQL for data hygiene if it exists.
		if tx.Migrator().HasTable("goals") {
			if err := tx.Exec("DELETE FROM goals WHERE user_id = ?", userID).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.Commitment{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.WheelOfLifeExercise{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorPlan{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorCheckIn{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.IdentityReflection{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.AcceptanceReflection{}).Error; err != nil {
			return err
		}

		// balance_transactions is NEVER deleted, and users.balance_seconds is never touched.
		// The ledger is the accounting record, and it stands on its own: signed amount,
		// running balance_after, session type and product — it needs nothing from the tables
		// purged around it. Deleting it while leaving balance_seconds in place (as this used
		// to) kept the balance but destroyed the statement that explains it.
		// The session_id values left dangling are harmless: there is no foreign key, and
		// every new session gets a fresh UUID, so the unique index still guarantees a
		// session can never be debited twice.

		if err := tx.Where("user_id = ?", userID).Delete(&models.CommitmentCompletion{}).Error; err != nil {
			return err
		}

		// Pending notifications go; the ones already delivered keep their telemetry
		// (scheduled_at, sent_at, and above all sent_via, which is how we know whether
		// chat or push reached the user) with the copy stripped.
		if err := redactNotifications(tx, userID); err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.Recommendation{}).Error; err != nil {
			return err
		}

		// Only today's growth snapshot: the historical rows are read by nothing and record
		// what was proposed on each day, which is analytics.
		if err := deleteTodaysGrowth(tx, userID, loc); err != nil {
			return err
		}

		// The session rows stay, so this stamp is what actually restarts the journey, the
		// streak and the session badges.
		if err := markJourneyReset(tx, userID); err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.UserBadge{}).Error; err != nil {
			return err
		}

		// Three things are deliberately NOT deleted here, all for the same reason: they are
		// not coaching data.
		//
		//   integrations   — a WhatsApp or Telegram binding is a communication channel.
		//                    Revoking it made "erase my data" silently disconnect the user's
		//                    chat, something they only notice days later when Rumi stops
		//                    replying, and re-linking is a deliberate act (POST .../link
		//                    refuses while one is active).
		//   user_devices   — a registered push token is the same class of thing as the
		//                    binding above, down to the platform label being analytics.
		//   user_app_opens — a user id and a date, nothing more. No code path reads it since
		//                    the streak moved to session history; it is simply the retention
		//                    record for the account, and erasing it bought nothing.
		//
		// Account deletion still removes the two live channels, where they genuinely must
		// not outlive the account.

		return nil
	}
}

// DeleteCurrentUser implements api.ServerInterface
func (s *Server) DeleteCurrentUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	anonEmail := fmt.Sprintf("%s@anonymized.com", userID)
	deletedName := "Deleted User"

	updates := map[string]interface{}{
		"email":                    anonEmail,
		"phone_number":             nil,
		"name":                     &deletedName,
		"date_of_birth":            nil,
		"gender":                   nil,
		"country":                  nil,
		"preferred_language":       nil,
		"ideal_life_vision":        nil,
		"top_values":               nil,
		"ideal_life_vision_set_at": nil,
		"focus_area":               nil,
		"is_active":                false,
		"latest_session_handle":    nil,
		"latest_session_handle_at": nil,
		"deleted_at":               time.Now(),
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.UserMemory{}).Error; err != nil {
			return err
		}

		// The message bodies and the session content used to survive account deletion
		// outright — the stronger action erased strictly less than the data reset did.
		// Sessions are redacted rather than deleted, keeping the usage and cost metrics
		// (duration, tokens, Deepgram seconds) that outlive any single account.
		if err := deleteChatScope(tx, userID); err != nil {
			return err
		}

		if err := deleteFeedbackScope(tx, userID); err != nil {
			return err
		}

		if err := redactSessionContent(tx, userID); err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.EisenhowerMatrixExercise{}).Error; err != nil {
			return err
		}

		// goals is a legacy orphaned table — purge via raw SQL for data hygiene if it exists.
		if tx.Migrator().HasTable("goals") {
			if err := tx.Exec("DELETE FROM goals WHERE user_id = ?", userID).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.Commitment{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.WheelOfLifeExercise{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorPlan{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.BehaviorCheckIn{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.IdentityReflection{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.AcceptanceReflection{}).Error; err != nil {
			return err
		}

		// balance_transactions is NEVER deleted, and users.balance_seconds is never touched.
		// The ledger is the accounting record, and it stands on its own: signed amount,
		// running balance_after, session type and product — it needs nothing from the tables
		// purged around it. Deleting it while leaving balance_seconds in place (as this used
		// to) kept the balance but destroyed the statement that explains it.
		// The session_id values left dangling are harmless: there is no foreign key, and
		// every new session gets a fresh UUID, so the unique index still guarantees a
		// session can never be debited twice.

		if err := tx.Where("user_id = ?", userID).Delete(&models.CommitmentCompletion{}).Error; err != nil {
			return err
		}

		if err := redactNotifications(tx, userID); err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.Recommendation{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.UserBadge{}).Error; err != nil {
			return err
		}

		// The two live channels DO die with the account: a push token and a chat binding
		// are open routes to a real person, and an account nobody can sign into must not
		// still be reachable. On a data reset they survive — see deleteAllUserData.
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserDevice{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", userID).Delete(&models.Integration{}).Error; err != nil {
			return err
		}

		// user_app_opens and the historical daily_growth rows stay: dates and proposals,
		// no content, and the retention record the product is measured by.

		return nil
	})

	if err != nil {
		s.logger.Error("failed to anonymize user account", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "Failed to anonymize account"}`, http.StatusInternalServerError)
		return
	}

	s.purgeFeedbackObjects(r.Context(), userID)

	// The identity record lives in the auth database: erase it too so the account can
	// no longer log in. The auth plane does this locally; remote data planes call the
	// auth plane's internal API. Local data is already gone; report an error if the
	// identity erasure fails so the client retries the deletion.
	if config.AppConfig.IsAuthPlane() {
		if err := eraseIdentity(userID); err != nil {
			s.logger.Error("failed to erase identity", zap.String("user_id", userID), zap.Error(err))
			http.Error(w, `{"error": "Account data deleted, but sign-in removal is pending. Please retry."}`, http.StatusInternalServerError)
			return
		}
	} else if cfg := config.AppConfig; cfg.RegionCode != "" && cfg.RegionCode != "eu" {
		if err := s.regional.DeleteIdentity(r.Context(), cfg.DataPlaneEUURL, userID); err != nil {
			s.logger.Error("failed to erase identity on auth plane", zap.String("user_id", userID), zap.Error(err))
			http.Error(w, `{"error": "Account data deleted, but sign-in removal is pending. Please retry."}`, http.StatusBadGateway)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetEisenhowerMatrix implements api.ServerInterface
func (s *Server) GetEisenhowerMatrix(w http.ResponseWriter, r *http.Request, params api.GetEisenhowerMatrixParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	page := 1
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}

	query := database.DB.Model(&models.EisenhowerMatrixExercise{}).Where("user_id = ?", userID)

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		s.logger.Error("Failed to count exercises", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to count exercises"}`, http.StatusInternalServerError)
		return
	}

	offset := (page - 1) * limit
	totalPages := int((totalItems + int64(limit) - 1) / int64(limit))

	var exercises []models.EisenhowerMatrixExercise
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&exercises).Error; err != nil {
		s.logger.Error("failed to fetch eisenhower matrix exercises", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "Failed to fetch exercises"}`, http.StatusInternalServerError)
		return
	}

	// Transform to API model
	apiExercises := make([]api.EisenhowerMatrixExercise, len(exercises))
	for i, ex := range exercises {
		var data api.EisenhowerMatrixData

		if err := json.Unmarshal([]byte(ex.Data), &data); err != nil {
			s.logger.Error("failed to unmarshal eisenhower matrix data", zap.String("exercise_id", ex.ID), zap.Error(err))
			continue
		}

		createdAt := ex.CreatedAt
		apiExercises[i] = api.EisenhowerMatrixExercise{
			Id:        &ex.ID,
			UserId:    &ex.UserID,
			SessionId: &ex.SessionID,
			Data:      &data,
			CreatedAt: &createdAt,
		}
	}

	resp := api.EisenhowerMatrixPaginatedResponse{
		Items: &apiExercises,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(totalItems)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetEisenhowerMatrixId implements api.ServerInterface
func (s *Server) GetEisenhowerMatrixId(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var ex models.EisenhowerMatrixExercise
	if err := database.DB.Where("id = ? AND user_id = ?", id, userID).First(&ex).Error; err != nil {
		http.Error(w, `{"error": "Exercise not found"}`, http.StatusNotFound)
		return
	}

	var data api.EisenhowerMatrixData
	if err := json.Unmarshal([]byte(ex.Data), &data); err != nil {
		s.logger.Error("failed to unmarshal eisenhower matrix data", zap.String("exercise_id", ex.ID), zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	createdAt := ex.CreatedAt
	apiEx := api.EisenhowerMatrixExercise{
		Id:        &ex.ID,
		UserId:    &ex.UserID,
		SessionId: &ex.SessionID,
		Data:      &data,
		CreatedAt: &createdAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiEx)
}

// RegisterFCMToken implements api.ServerInterface
func (s *Server) RegisterFCMToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req api.FCMTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, `{"error": "FCM Token is required"}`, http.StatusBadRequest)
		return
	}

	var platform string
	if req.Platform != nil {
		platform = *req.Platform
	}

	var device models.UserDevice
	err := database.DB.Where("fcm_token = ?", req.Token).First(&device).Error
	if err == nil {
		// Token exists, update owner and platform
		device.UserID = userID
		device.Platform = platform
		if err := database.DB.Save(&device).Error; err != nil {
			s.logger.Error("failed to update user device", zap.Error(err))
			http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
			return
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// New token, insert
		newDevice := models.UserDevice{
			UserID:   userID,
			FCMToken: req.Token,
			Platform: platform,
		}
		if err := database.DB.Create(&newDevice).Error; err != nil {
			s.logger.Error("failed to create user device", zap.Error(err))
			http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
			return
		}
	} else {
		s.logger.Error("failed to query user device", zap.Error(err))
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
