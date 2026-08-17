package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/internal/services/regional"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// sessionCostUSDSQL is the per-session cost in USD, read from the cost ledger rather
// than recomputed. It references the communication_sessions row aliased as "c".
//
// This used to be the pricing arithmetic itself, with the rates written out as floats
// in this string. They happened to be right, but they were a second copy of numbers
// that live in aiusage/prices.go, in a form no test could check and no price change
// would touch — and they only ever covered the live session, so the recap, the QA
// review and the recommendation agent that every session also pays for were missing
// from every figure on this dashboard.
//
// Summing the ledger fixes both: it prices per modality from the one table that knows
// the rates, and it picks up every call attributed to the session. cost_micros is
// NULL for a model the price table does not know, and such rows are skipped rather
// than counted as zero — an unpriced model should leave the number visibly short, not
// silently understate it.
const sessionCostUSDSQL = `(
	SELECT COALESCE(SUM(a.cost_micros), 0)::NUMERIC / 1000000
	FROM ai_usage_records a
	WHERE a.ref_type = 'communication_session' AND a.ref_id = c.id
)`

// sessionTokensSQL builds a per-session token total from the cost ledger, for one of
// its modality columns.
func sessionTokensSQL(column string) string {
	return fmt.Sprintf(`(
		SELECT COALESCE(SUM(a.%s), 0)
		FROM ai_usage_records a
		WHERE a.ref_type = 'communication_session' AND a.ref_id = c.id
	)`, column)
}

// sessionDurationMinSQL is the per-session duration in minutes, NULL for sessions that have
// not ended yet (so averages/sums ignore in-progress sessions).
const sessionDurationMinSQL = `CASE WHEN c.end_time IS NOT NULL
	THEN EXTRACT(EPOCH FROM (c.end_time - c.start_time)) / 60.0 END`

type AdminService struct {
	logger *zap.Logger
	// regional provisions profiles for waitlist identities at activation time
	// (waitlist signups defer provisioning; see auth.createAccount).
	regional *regional.Client
}

var Admin *AdminService

func InitAdminService(logger *zap.Logger) {
	Admin = NewAdminService(logger)
}

func NewAdminService(logger *zap.Logger) *AdminService {
	return &AdminService{logger: logger, regional: regional.NewClient()}
}

func (s *AdminService) ListUsers(ctx context.Context, page, limit int, params api.GetAdminUsersParams) ([]models.User, int64, error) {
	// Waitlist identities awaiting provisioning exist only on the auth plane; they
	// are listed ahead of provisioned users so operators see who is waiting.
	pending, err := s.pendingWaitlistUsers(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list pending waitlist identities", zap.Error(err))
		return nil, 0, err
	}

	// waitlisted=true: the waiting list alone, paginated from the pending slice.
	if params.Waitlisted != nil && *params.Waitlisted {
		offset := (page - 1) * limit
		if offset > len(pending) {
			offset = len(pending)
		}
		end := offset + limit
		if end > len(pending) {
			end = len(pending)
		}
		return pending[offset:end], int64(len(pending)), nil
	}

	var total int64
	db := database.DB.WithContext(ctx).Model(&models.User{})

	// Exclude deleted users (deleted users have their email set to ID@anonymized.com)
	db = db.Where("email IS NULL OR email NOT LIKE ?", "%@anonymized.com")

	if params.Name != nil && *params.Name != "" {
		db = db.Where("name ILIKE ?", "%"+*params.Name+"%")
	}
	if params.Country != nil && *params.Country != "" {
		db = db.Where("country ILIKE ?", "%"+*params.Country+"%")
	}
	if params.PreferredLanguage != nil && *params.PreferredLanguage != "" {
		db = db.Where("preferred_language = ?", *params.PreferredLanguage)
	}
	if params.IsActive != nil {
		db = db.Where("is_active = ?", *params.IsActive)
	}
	if params.CreatedAt != nil {
		// assuming Date maps to start of day
		db = db.Where("DATE(created_at) = ?", params.CreatedAt.Time.Format("2006-01-02"))
	}

	if err := db.Count(&total).Error; err != nil {
		s.logger.Error("Failed to count users", zap.Error(err))
		return nil, 0, err
	}
	total += int64(len(pending))

	// The combined list is pending-first: page slots [0, len(pending)) come from the
	// waitlist, the rest from the users table with the offset shifted accordingly.
	offset := (page - 1) * limit
	users := make([]models.User, 0, limit)
	if offset < len(pending) {
		end := offset + limit
		if end > len(pending) {
			end = len(pending)
		}
		users = append(users, pending[offset:end]...)
	}
	if remaining := limit - len(users); remaining > 0 {
		dbOffset := offset - len(pending)
		if dbOffset < 0 {
			dbOffset = 0
		}
		var provisioned []models.User
		if err := db.Order("created_at desc").Offset(dbOffset).Limit(remaining).Find(&provisioned).Error; err != nil {
			s.logger.Error("Failed to list users", zap.Error(err))
			return nil, 0, err
		}
		users = append(users, provisioned...)
	}

	return users, total, nil
}

// pendingWaitlistUsers returns synthesized User rows for waitlist identities whose
// regional profile has not been provisioned yet, applying the same filters ListUsers
// applies to provisioned users. Name/date filter in SQL; country/language live inside
// the serialized provision, so they filter after unmarshaling.
func (s *AdminService) pendingWaitlistUsers(ctx context.Context, params api.GetAdminUsersParams) ([]models.User, error) {
	if params.IsActive != nil && *params.IsActive {
		return nil, nil // a pending identity is by definition not active
	}
	if params.Waitlisted != nil && !*params.Waitlisted {
		return nil, nil // provisioned users only
	}

	q := database.Auth().WithContext(ctx).Where("pending_provision IS NOT NULL")
	if params.Name != nil && *params.Name != "" {
		q = q.Where("name ILIKE ?", "%"+*params.Name+"%")
	}
	if params.CreatedAt != nil {
		q = q.Where("DATE(created_at) = ?", params.CreatedAt.Time.Format("2006-01-02"))
	}

	var identities []models.Identity
	if err := q.Order("created_at desc").Find(&identities).Error; err != nil {
		return nil, err
	}

	users := make([]models.User, 0, len(identities))
	for i := range identities {
		u, err := userFromPendingIdentity(&identities[i])
		if err != nil {
			s.logger.Warn("Skipping unreadable pending provision", zap.String("user_id", identities[i].ID), zap.Error(err))
			continue
		}
		if params.Country != nil && *params.Country != "" &&
			(u.Country == nil || !strings.Contains(strings.ToLower(*u.Country), strings.ToLower(*params.Country))) {
			continue
		}
		if params.PreferredLanguage != nil && *params.PreferredLanguage != "" &&
			(u.PreferredLanguage == nil || *u.PreferredLanguage != *params.PreferredLanguage) {
			continue
		}
		users = append(users, *u)
	}
	return users, nil
}

// userFromPendingIdentity builds the User view of a waitlist identity from its
// stored provision payload, for admin listing/inspection before the real profile
// row exists.
func userFromPendingIdentity(identity *models.Identity) (*models.User, error) {
	var p regional.UserProvision
	if err := json.Unmarshal([]byte(*identity.PendingProvision), &p); err != nil {
		return nil, err
	}
	u := userFromProvision(&p, identity.CreatedAt)
	u.IsActive = identity.IsActive
	u.Waitlisted = true
	return u, nil
}

func userFromProvision(p *regional.UserProvision, createdAt time.Time) *models.User {
	dataRegion := p.DataRegion
	return &models.User{
		ID:                           p.ID,
		Email:                        p.Email,
		PhoneNumber:                  p.PhoneNumber,
		Name:                         p.Name,
		DateOfBirth:                  p.DateOfBirth,
		Gender:                       p.Gender,
		Country:                      p.Country,
		Region:                       p.Region,
		DataRegion:                   &dataRegion,
		PreferredLanguage:            p.PreferredLanguage,
		IsActive:                     p.IsActive,
		TermsAndConditionsAcceptedAt: p.TermsAndConditionsAcceptedAt,
		MarketingAcceptedAt:          p.MarketingAcceptedAt,
		AIAcceptedAt:                 p.AIAcceptedAt,
		CreatedAt:                    createdAt,
	}
}

func (s *AdminService) GetUser(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	db := database.DB.WithContext(ctx)

	err := db.First(&user, "id = ?", id).Error
	if err == nil {
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// No profile row: the ID may be a waitlist identity whose provisioning is
	// deferred until activation.
	var identity models.Identity
	if err := database.Auth().WithContext(ctx).First(&identity, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if identity.PendingProvision == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return userFromPendingIdentity(&identity)
}

func (s *AdminService) SetUserActiveStatus(ctx context.Context, id string, isActive bool) (*models.User, error) {
	authDB := database.Auth().WithContext(ctx)
	var identity models.Identity
	if err := authDB.First(&identity, "id = ?", id).Error; err != nil {
		return nil, err
	}

	// Waitlist identity whose regional profile was never provisioned: activation is
	// the moment the profile row gets created. Provisioning is idempotent and the
	// pending payload is only cleared on success, so a failed activation is simply
	// retried by the admin.
	if identity.PendingProvision != nil {
		user, err := userFromPendingIdentity(&identity)
		if err != nil {
			return nil, fmt.Errorf("unreadable pending provision for %s: %w", id, err)
		}
		if !isActive {
			return user, nil // still waitlisted; there is no profile row to touch
		}

		var p regional.UserProvision
		if err := json.Unmarshal([]byte(*identity.PendingProvision), &p); err != nil {
			return nil, err
		}
		p.IsActive = true
		if err := s.regional.ProvisionInRegion(ctx, p.DataRegion, p); err != nil {
			s.logger.Error("Activation provisioning failed", zap.Error(err),
				zap.String("user_id", id), zap.String("data_region", p.DataRegion))
			return nil, err
		}
		if err := authDB.Model(&models.Identity{}).Where("id = ?", id).
			Updates(map[string]interface{}{"is_active": true, "pending_provision": nil}).Error; err != nil {
			s.logger.Error("Failed to update identity status", zap.Error(err), zap.String("user_id", id))
			return nil, err
		}

		user.IsActive = true
		user.Waitlisted = false // provisioned just now
		s.notifyActivated(*user)
		return user, nil
	}

	var user models.User
	db := database.DB.WithContext(ctx)

	if err := db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}

	wasActive := user.IsActive
	user.IsActive = isActive
	// balance_seconds is owned by the balance ledger; the model marks it <-:false,
	// so this full-row save skips it and cannot clobber a concurrent session debit.
	if err := db.Save(&user).Error; err != nil {
		s.logger.Error("Failed to update user status", zap.Error(err), zap.String("user_id", id))
		return nil, err
	}

	// Login eligibility is decided by the identity record in the auth database —
	// keep it in sync (admin runs on the auth plane).
	if err := authDB.Model(&models.Identity{}).
		Where("id = ?", id).Update("is_active", isActive).Error; err != nil {
		s.logger.Error("Failed to update identity status", zap.Error(err), zap.String("user_id", id))
		return nil, err
	}

	// Trigger notifications if the user is being activated for the first time
	if !wasActive && isActive {
		s.notifyActivated(user)
	}

	return &user, nil
}

// notifyActivated sends the account-activated email/SMS in the background.
func (s *AdminService) notifyActivated(user models.User) {
	if notification.GlobalNotificationService == nil {
		return
	}
	// Run in a goroutine so we don't block the admin response
	go func(u models.User) {
		name := u.Name
		if name == nil || *name == "" {
			defaultName := "User"
			name = &defaultName
		}

		locale := "en-US"
		if u.PreferredLanguage != nil && *u.PreferredLanguage != "" {
			locale = *u.PreferredLanguage
		}

		// Send Email if available
		if u.Email != nil && *u.Email != "" {
			if err := notification.GlobalNotificationService.SendAccountActivatedEmail(*u.Email, *name, locale); err != nil {
				s.logger.Error("Failed to send account activated email", zap.Error(err), zap.String("user_id", u.ID))
			}
		}

		// Send SMS if available
		if u.PhoneNumber != nil && *u.PhoneNumber != "" {
			if err := notification.GlobalNotificationService.SendAccountActivatedSMS(*u.PhoneNumber, *name, locale); err != nil {
				s.logger.Error("Failed to send account activated SMS", zap.Error(err), zap.String("user_id", u.ID))
			}
		}
	}(user)
}

func (s *AdminService) ListSessions(ctx context.Context, page, limit int, params api.GetAdminSessionsParams, minDuration, maxDuration *float64) ([]api.CommunicationSessionAdmin, int64, error) {
	var items []api.CommunicationSessionAdmin
	var total int64

	offset := (page - 1) * limit
	db := database.DB.WithContext(ctx).Table("communication_sessions c").
		Joins("LEFT JOIN users u ON c.user_id = u.id")

	if params.UserId != nil && *params.UserId != "" {
		db = db.Where("c.user_id = ?", *params.UserId)
	}
	if params.UserName != nil && *params.UserName != "" {
		db = db.Where("u.name ILIKE ?", "%"+*params.UserName+"%")
	}
	if params.UserCountry != nil && *params.UserCountry != "" {
		db = db.Where("u.country ILIKE ?", "%"+*params.UserCountry+"%")
	}
	if params.UserLanguage != nil && *params.UserLanguage != "" {
		db = db.Where("u.preferred_language = ?", *params.UserLanguage)
	}
	if minDuration != nil {
		db = db.Where("c.end_time IS NOT NULL AND EXTRACT(EPOCH FROM (c.end_time - c.start_time)) / 60 >= ?", *minDuration)
	}
	if maxDuration != nil {
		db = db.Where("c.end_time IS NOT NULL AND EXTRACT(EPOCH FROM (c.end_time - c.start_time)) / 60 <= ?", *maxDuration)
	}

	if err := db.Count(&total).Error; err != nil {
		s.logger.Error("Failed to count communication sessions", zap.Error(err))
		return nil, 0, err
	}

	// Tokens and cost both come from the ledger — this query used to carry its own
	// copy of the pricing arithmetic, which drifted from the dashboard's the moment
	// either was edited.
	selectClause := fmt.Sprintf(`
		c.id, c.user_id, u.name AS user_name, u.country AS user_country, 
		u.preferred_language AS user_language, c.start_time, c.end_time, c.language, c.session_type,
		%s AS input_text_tokens, %s AS output_text_tokens,
		%s AS input_audio_tokens, %s AS output_audio_tokens,
		c.ai_notes, c.ai_evaluation, c.user_evaluation, c.user_feedback, c.user_session_insight,
		ROUND(%s, 6) AS total_cost_usd
	`,
		sessionTokensSQL("input_text_tokens"), sessionTokensSQL("output_text_tokens"),
		sessionTokensSQL("input_audio_tokens"), sessionTokensSQL("output_audio_tokens"),
		sessionCostUSDSQL)

	if err := db.Select(selectClause).Order("c.start_time DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		s.logger.Error("Failed to fetch admin sessions", zap.Error(err))
		return nil, 0, err
	}

	return items, total, nil
}

// Dashboard returns aggregate CEO/PM metrics over users and communication sessions. The
// optional [from, to] window scopes session-based metrics (sessions, cost, quality, and active/
// returning/new users); user totals and the onboarding funnel are current snapshots.
func (s *AdminService) Dashboard(ctx context.Context, from, to *time.Time) (*api.AdminDashboard, error) {
	db := database.DB.WithContext(ctx)

	// Session window (sessions aliased as "c").
	swhere := "WHERE 1=1"
	var sargs []interface{}
	if from != nil {
		swhere += " AND c.start_time >= ?"
		sargs = append(sargs, *from)
	}
	if to != nil {
		swhere += " AND c.start_time <= ?"
		sargs = append(sargs, *to)
	}

	// --- Session + quality + cost aggregate ---
	var agg struct {
		TotalSessions             int
		ActiveUsers               int
		TotalCostUsd              float64
		TotalDurationMinutes      float64
		AvgSessionDurationMinutes float64
		AvgCostPerSessionUsd      float64
		AvgCostPerMinuteUsd       float64
		AvgAiEvaluation           float64
		AvgUserEvaluation         float64
		RatedSessions             int
		ReviewedSessions          int
	}
	aggSQL := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_sessions,
			COUNT(DISTINCT c.user_id) AS active_users,
			COALESCE(SUM(%[1]s), 0) AS total_cost_usd,
			COALESCE(SUM(%[2]s), 0) AS total_duration_minutes,
			COALESCE(AVG(%[2]s), 0) AS avg_session_duration_minutes,
			COALESCE(AVG(%[1]s), 0) AS avg_cost_per_session_usd,
			CASE WHEN COALESCE(SUM(%[2]s), 0) > 0
				THEN COALESCE(SUM(CASE WHEN c.end_time IS NOT NULL THEN %[1]s END), 0) / SUM(%[2]s)
				ELSE 0 END AS avg_cost_per_minute_usd,
			COALESCE(AVG(c.ai_evaluation), 0) AS avg_ai_evaluation,
			COALESCE(AVG(c.user_evaluation), 0) AS avg_user_evaluation,
			COUNT(c.user_evaluation) AS rated_sessions,
			COUNT(c.ai_evaluation) AS reviewed_sessions
		FROM communication_sessions c
		%[3]s
	`, sessionCostUSDSQL, sessionDurationMinSQL, swhere)
	if err := db.Raw(aggSQL, sargs...).Scan(&agg).Error; err != nil {
		s.logger.Error("dashboard: session aggregate failed", zap.Error(err))
		return nil, err
	}

	// Returning users: distinct users with >= 2 sessions in the window.
	var returningUsers int
	retSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT c.user_id FROM communication_sessions c
			%s
			GROUP BY c.user_id
			HAVING COUNT(*) >= 2
		) t
	`, swhere)
	if err := db.Raw(retSQL, sargs...).Scan(&returningUsers).Error; err != nil {
		s.logger.Error("dashboard: returning users failed", zap.Error(err))
		return nil, err
	}

	// --- Sessions by type ---
	var typeRows []struct {
		SessionType        *string
		SessionCount       int
		AvgDurationMinutes float64
		AvgCostUsd         float64
		TotalCostUsd       float64
	}
	typeSQL := fmt.Sprintf(`
		SELECT
			c.session_type AS session_type,
			COUNT(*) AS session_count,
			COALESCE(AVG(%[2]s), 0) AS avg_duration_minutes,
			COALESCE(AVG(%[1]s), 0) AS avg_cost_usd,
			COALESCE(SUM(%[1]s), 0) AS total_cost_usd
		FROM communication_sessions c
		%[3]s
		GROUP BY c.session_type
		ORDER BY session_count DESC
	`, sessionCostUSDSQL, sessionDurationMinSQL, swhere)
	if err := db.Raw(typeSQL, sargs...).Scan(&typeRows).Error; err != nil {
		s.logger.Error("dashboard: sessions by type failed", zap.Error(err))
		return nil, err
	}

	// --- User totals + onboarding funnel (current snapshot) ---
	var userAgg struct {
		TotalUsers           int
		OnboardingInProgress int
		OnboardingCompleted  int
	}
	if err := db.Raw(`
		SELECT
			COUNT(*) AS total_users,
			COUNT(*) FILTER (WHERE state LIKE 'ONBOARDING%' OR state LIKE 'VISION\_%') AS onboarding_in_progress,
			COUNT(*) FILTER (WHERE state IS NOT NULL AND state NOT LIKE 'ONBOARDING%' AND state NOT LIKE 'VISION\_%') AS onboarding_completed
		FROM users
	`).Scan(&userAgg).Error; err != nil {
		s.logger.Error("dashboard: user totals failed", zap.Error(err))
		return nil, err
	}

	// New users: created within the window if given, otherwise within the last 30 days.
	var newUsers int
	newSQL := "SELECT COUNT(*) FROM users WHERE 1=1"
	var nargs []interface{}
	if from != nil || to != nil {
		if from != nil {
			newSQL += " AND created_at >= ?"
			nargs = append(nargs, *from)
		}
		if to != nil {
			newSQL += " AND created_at <= ?"
			nargs = append(nargs, *to)
		}
	} else {
		newSQL += " AND created_at >= NOW() - INTERVAL '30 days'"
	}
	if err := db.Raw(newSQL, nargs...).Scan(&newUsers).Error; err != nil {
		s.logger.Error("dashboard: new users failed", zap.Error(err))
		return nil, err
	}

	// --- User demographics (over all users) ---
	userDemographic := func(keyExpr string) ([]api.AdminDemographicStat, error) {
		var rows []struct {
			Key       string
			UserCount int
		}
		q := fmt.Sprintf(`
			SELECT COALESCE(%[1]s, 'unknown') AS key, COUNT(*) AS user_count
			FROM users
			GROUP BY COALESCE(%[1]s, 'unknown')
			ORDER BY user_count DESC
		`, keyExpr)
		if err := db.Raw(q).Scan(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]api.AdminDemographicStat, 0, len(rows))
		for _, r := range rows {
			out = append(out, api.AdminDemographicStat{Key: ptrStr(r.Key), UserCount: ptrInt(r.UserCount)})
		}
		return out, nil
	}

	ageBucketExpr := `CASE
		WHEN date_of_birth IS NULL THEN 'unknown'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 18 THEN 'under_18'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 25 THEN '18-24'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 35 THEN '25-34'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 45 THEN '35-44'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 55 THEN '45-54'
		WHEN EXTRACT(YEAR FROM AGE(date_of_birth)) < 65 THEN '55-64'
		ELSE '65_plus' END`

	byCountry, err := userDemographic("country")
	if err != nil {
		s.logger.Error("dashboard: users by country failed", zap.Error(err))
		return nil, err
	}
	byGender, err := userDemographic("gender")
	if err != nil {
		s.logger.Error("dashboard: users by gender failed", zap.Error(err))
		return nil, err
	}
	byLanguage, err := userDemographic("preferred_language")
	if err != nil {
		s.logger.Error("dashboard: users by language failed", zap.Error(err))
		return nil, err
	}
	byAge, err := userDemographic(ageBucketExpr)
	if err != nil {
		s.logger.Error("dashboard: users by age failed", zap.Error(err))
		return nil, err
	}

	// --- Assemble ---
	typeStats := make([]api.AdminSessionTypeStat, 0, len(typeRows))
	for _, t := range typeRows {
		typeStats = append(typeStats, api.AdminSessionTypeStat{
			SessionType:        t.SessionType,
			SessionCount:       ptrInt(t.SessionCount),
			AvgDurationMinutes: ptrF32(t.AvgDurationMinutes),
			AvgCostUsd:         ptrF32(t.AvgCostUsd),
			TotalCostUsd:       ptrF32(t.TotalCostUsd),
		})
	}

	avgSessionsPerActiveUser := 0.0
	avgCostPerUser := 0.0
	if agg.ActiveUsers > 0 {
		avgSessionsPerActiveUser = float64(agg.TotalSessions) / float64(agg.ActiveUsers)
		avgCostPerUser = agg.TotalCostUsd / float64(agg.ActiveUsers)
	}

	return &api.AdminDashboard{
		PeriodFrom: from,
		PeriodTo:   to,
		Users: &api.AdminDashboardUsers{
			TotalUsers:               ptrInt(userAgg.TotalUsers),
			NewUsers:                 ptrInt(newUsers),
			ActiveUsers:              ptrInt(agg.ActiveUsers),
			ReturningUsers:           ptrInt(returningUsers),
			AvgSessionsPerActiveUser: ptrF32(avgSessionsPerActiveUser),
			OnboardingCompleted:      ptrInt(userAgg.OnboardingCompleted),
			OnboardingInProgress:     ptrInt(userAgg.OnboardingInProgress),
			Demographics: &api.AdminUserDemographics{
				ByCountry:   &byCountry,
				ByGender:    &byGender,
				ByLanguage:  &byLanguage,
				ByAgeBucket: &byAge,
			},
		},
		Sessions: &api.AdminDashboardSessions{
			TotalSessions:             ptrInt(agg.TotalSessions),
			TotalDurationMinutes:      ptrF32(agg.TotalDurationMinutes),
			AvgSessionDurationMinutes: ptrF32(agg.AvgSessionDurationMinutes),
			BySessionType:             &typeStats,
		},
		Quality: &api.AdminDashboardQuality{
			AvgAiEvaluation:   ptrF32(agg.AvgAiEvaluation),
			AvgUserEvaluation: ptrF32(agg.AvgUserEvaluation),
			RatedSessions:     ptrInt(agg.RatedSessions),
			ReviewedSessions:  ptrInt(agg.ReviewedSessions),
		},
		Cost: &api.AdminDashboardCost{
			TotalCostUsd:         ptrF32(agg.TotalCostUsd),
			AvgCostPerSessionUsd: ptrF32(agg.AvgCostPerSessionUsd),
			AvgCostPerMinuteUsd:  ptrF32(agg.AvgCostPerMinuteUsd),
			AvgCostPerUserUsd:    ptrF32(avgCostPerUser),
		},
	}, nil
}

func ptrInt(i int) *int       { return &i }
func ptrStr(s string) *string { return &s }
func ptrF32(f float64) *float32 {
	v := float32(f)
	return &v
}
