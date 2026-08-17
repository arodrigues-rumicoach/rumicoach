package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// The export is built from purpose-written view structs, one per record type, rather than
// by marshalling the GORM models and stripping fields afterwards. The previous approach
// deleted any key whose name ended in "Id"/"ID"/"_id", which failed in both directions: it
// removed things the user should have (a debit's link to its session) and it could not see
// anything else — a new column on any model landed in the export the day it was added, in
// silence. A whitelist cannot leak what nobody put in it.
//
// The rule for what belongs here: data the user gave us or that is about them. Not our
// surrogate keys, not live credentials (push tokens, link codes), not internal machinery
// (session state, resumption handles, routing flags) that means nothing to a reader.

type exportedUser struct {
	Name              *string    `json:"name,omitempty"`
	Email             *string    `json:"email,omitempty"`
	PhoneNumber       *string    `json:"phoneNumber,omitempty"`
	DateOfBirth       *time.Time `json:"dateOfBirth,omitempty"`
	Gender            *string    `json:"gender,omitempty"`
	Country           *string    `json:"country,omitempty"`
	PreferredLanguage *string    `json:"preferredLanguage,omitempty"`
	// DataRegion is where the account's data physically lives. Kept deliberately: it is a
	// transparency fact about their own data, not routing trivia.
	DataRegion                   *string    `json:"dataRegion,omitempty"`
	CoachGender                  *string    `json:"coachGender,omitempty"`
	CoachVoice                   *string    `json:"coachVoice,omitempty"`
	Theme                        *string    `json:"theme,omitempty"`
	IdealLifeVision              *string    `json:"idealLifeVision,omitempty"`
	IdealLifeVisionSetAt         *time.Time `json:"idealLifeVisionSetAt,omitempty"`
	FocusArea                    *string    `json:"focusArea,omitempty"`
	BalanceSeconds               int64      `json:"balanceSeconds"`
	TermsAndConditionsAcceptedAt *time.Time `json:"termsAndConditionsAcceptedAt,omitempty"`
	MarketingAcceptedAt          *time.Time `json:"marketingAcceptedAt,omitempty"`
	AIAcceptedAt                 *time.Time `json:"aiAcceptedAt,omitempty"`
	CreatedAt                    time.Time  `json:"createdAt"`
}

// exportedDevice carries no FCM token. The token is a live push credential with no meaning
// to the user, and an export is a file they save and may well share.
type exportedDevice struct {
	Platform     string    `json:"platform,omitempty"`
	RegisteredAt time.Time `json:"registeredAt"`
}

// exportedIntegration describes the connection without the identifiers that make it work:
// no binding id, no provider-side external id, and above all no link code, which is a live
// one-time credential.
type exportedIntegration struct {
	Provider             string     `json:"provider"`
	Status               string     `json:"status"`
	ReplyMode            string     `json:"replyMode,omitempty"`
	ConnectedAt          time.Time  `json:"connectedAt"`
	LastMessageFromYouAt *time.Time `json:"lastMessageFromYouAt,omitempty"`
}

// exportedCommitment nests its completion dates. Completions carry only a commitment id
// and a date, so stripped of the id they were a list of bare dates meaning nothing;
// nesting them says what was actually kept, and needs no identifier at all.
type exportedCommitment struct {
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	Origin      string          `json:"origin"`
	Days        models.IntSlice `json:"days,omitempty"`
	Date        *string         `json:"date,omitempty"`
	Done        bool            `json:"done"`
	EndedAt     *time.Time      `json:"endedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedOn []string        `json:"completedOn,omitempty"`
}

// exportedBehaviorPlan nests its check-ins, for the same reason.
type exportedBehaviorPlan struct {
	Status        string                    `json:"status"`
	Behavior      string                    `json:"behavior"`
	Identity      string                    `json:"identity,omitempty"`
	Motive        string                    `json:"motive,omitempty"`
	Trigger       string                    `json:"trigger,omitempty"`
	Context       string                    `json:"context,omitempty"`
	Frequency     string                    `json:"frequency,omitempty"`
	Days          models.IntSlice           `json:"days,omitempty"`
	Obstacles     string                    `json:"obstacles,omitempty"`
	PlanB         string                    `json:"planB,omitempty"`
	Area          *string                   `json:"area,omitempty"`
	StartDate     *string                   `json:"startDate,omitempty"`
	WinsCount     int                       `json:"winsCount"`
	LastCheckInAt *time.Time                `json:"lastCheckInAt,omitempty"`
	CreatedAt     time.Time                 `json:"createdAt"`
	CheckIns      []exportedBehaviorCheckIn `json:"checkIns,omitempty"`
}

type exportedBehaviorCheckIn struct {
	Status    string    `json:"status"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// exportedIdentityReflection is the Identity session's structured synthesis — the
// user's own words, so it belongs in their export.
type exportedIdentityReflection struct {
	LearnedIdentity string             `json:"learnedIdentity"`
	WhatItGave      string             `json:"whatItGave,omitempty"`
	WhatItCosts     string             `json:"whatItCosts,omitempty"`
	WhoBecoming     string             `json:"whoBecoming"`
	Qualities       models.StringSlice `json:"qualities,omitempty"`
	Evidence        string             `json:"evidence,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
}

// exportedAcceptanceReflection is the Acceptance session's structured synthesis — the
// user's own words, so it belongs in their export.
type exportedAcceptanceReflection struct {
	Expected       string    `json:"expected"`
	Reality        string    `json:"reality"`
	CannotControl  string    `json:"cannotControl,omitempty"`
	CanInfluence   string    `json:"canInfluence,omitempty"`
	ChooseToAccept string    `json:"chooseToAccept,omitempty"`
	WhereIAct      string    `json:"whereIAct,omitempty"`
	NextStep       string    `json:"nextStep,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// exportedSession omits the transcript, and that omission is deliberate: transcripts are
// retained only long enough to generate the recap and the review, and are on their way out
// of the product entirely. It also omits ai_notes and ai_evaluation, which grade the
// coach's conduct rather than record the user.
type exportedSession struct {
	StartTime          time.Time  `json:"startTime"`
	EndTime            *time.Time `json:"endTime,omitempty"`
	Duration           int        `json:"duration"`
	Language           *string    `json:"language,omitempty"`
	SessionType        *string    `json:"sessionType,omitempty"`
	UserEvaluation     *float64   `json:"userEvaluation,omitempty"`
	UserFeedback       *string    `json:"userFeedback,omitempty"`
	UserSessionInsight *string    `json:"userSessionInsight,omitempty"`
	SessionSummary     *string    `json:"sessionSummary,omitempty"`
	Recap              *string    `json:"recap,omitempty"`
	RecapTitle         *string    `json:"recapTitle,omitempty"`
}

type exportedChannelMessage struct {
	Provider  string    `json:"provider"`
	Direction string    `json:"direction"`
	Type      string    `json:"type"`
	Body      *string   `json:"body,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type exportedMemory struct {
	Category  string    `json:"category"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// exportedExercise covers both the Wheel of Life and the Eisenhower matrix: a timestamp
// and the stored JSON payload, without the session key that produced it.
type exportedExercise struct {
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
}

type exportedTransaction struct {
	Type          string    `json:"type"`
	AmountSeconds int64     `json:"amountSeconds"`
	BalanceAfter  int64     `json:"balanceAfter"`
	SessionType   *string   `json:"sessionType,omitempty"`
	Product       *string   `json:"product,omitempty"`
	Description   *string   `json:"description,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type exportedBadge struct {
	Badge    string    `json:"badge"`
	EarnedAt time.Time `json:"earnedAt"`
}

// exportedFeedback carries the report and how many images went with it, but no link to
// them: a signed URL would be stale by the time anyone opened the file, and an unsigned
// path is meaningless outside our bucket. The count is the honest thing to say.
type exportedFeedback struct {
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Platform    *string   `json:"platform,omitempty"`
	AppVersion  *string   `json:"appVersion,omitempty"`
	Images      int       `json:"images"`
	CreatedAt   time.Time `json:"createdAt"`
}

type exportedRecommendation struct {
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	Author      *string   `json:"author,omitempty"`
	URL         *string   `json:"url,omitempty"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ExportCurrentUserData implements api.ServerInterface: the synchronous download.
func (s *Server) ExportCurrentUserData(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="rumicoach_export.json"`)

	if err := json.NewEncoder(w).Encode(buildUserDataExport(database.DB, userID)); err != nil {
		s.logger.Error("failed to encode export data", zap.String("user_id", userID), zap.Error(err))
		return
	}
}

// buildUserDataExport assembles the whole export. Shared by the download route and the
// emailed one so the two can never drift into disagreeing about what a user's data is.
//
// Individual query failures degrade to an omitted block rather than a failed export: a
// partial copy of your data beats an error page, and the blocks are independent.
func buildUserDataExport(db *gorm.DB, userID string) map[string]interface{} {
	export := make(map[string]interface{})

	var user models.User
	if err := db.Where("id = ?", userID).First(&user).Error; err == nil {
		export["user"] = exportedUser{
			Name:                         user.Name,
			Email:                        user.Email,
			PhoneNumber:                  user.PhoneNumber,
			DateOfBirth:                  user.DateOfBirth,
			Gender:                       user.Gender,
			Country:                      user.Country,
			PreferredLanguage:            user.PreferredLanguage,
			DataRegion:                   user.DataRegion,
			CoachGender:                  user.CoachGender,
			CoachVoice:                   user.CoachVoice,
			Theme:                        user.Theme,
			IdealLifeVision:              user.IdealLifeVision,
			IdealLifeVisionSetAt:         user.IdealLifeVisionSetAt,
			FocusArea:                    user.FocusArea,
			BalanceSeconds:               user.BalanceSeconds,
			TermsAndConditionsAcceptedAt: user.TermsAndConditionsAcceptedAt,
			MarketingAcceptedAt:          user.MarketingAcceptedAt,
			AIAcceptedAt:                 user.AIAcceptedAt,
			CreatedAt:                    user.CreatedAt,
		}
	}

	var devices []models.UserDevice
	if err := db.Where("user_id = ?", userID).Find(&devices).Error; err == nil && len(devices) > 0 {
		out := make([]exportedDevice, 0, len(devices))
		for _, d := range devices {
			out = append(out, exportedDevice{Platform: d.Platform, RegisteredAt: d.CreatedAt})
		}
		export["devices"] = out
	}

	var integrations []models.Integration
	if err := db.Where("user_id = ?", userID).Find(&integrations).Error; err == nil && len(integrations) > 0 {
		out := make([]exportedIntegration, 0, len(integrations))
		for _, i := range integrations {
			out = append(out, exportedIntegration{
				Provider:             i.Provider,
				Status:               i.Status,
				ReplyMode:            i.ReplyMode,
				ConnectedAt:          i.CreatedAt,
				LastMessageFromYouAt: i.LastInboundAt,
			})
		}
		export["integrations"] = out
	}

	// Completions are grouped onto their commitment, so nothing needs an identifier.
	completionsBy := map[string][]string{}
	var completions []models.CommitmentCompletion
	if err := db.Where("user_id = ?", userID).Order("date").Find(&completions).Error; err == nil {
		for _, c := range completions {
			completionsBy[c.CommitmentID] = append(completionsBy[c.CommitmentID], c.Date)
		}
	}

	var commitments []models.Commitment
	if err := db.Where("user_id = ?", userID).Find(&commitments).Error; err == nil && len(commitments) > 0 {
		out := make([]exportedCommitment, 0, len(commitments))
		for _, c := range commitments {
			out = append(out, exportedCommitment{
				Title:       c.Title,
				Type:        c.Type,
				Origin:      c.Origin,
				Days:        c.Days,
				Date:        c.Date,
				Done:        c.Done,
				EndedAt:     c.EndedAt,
				CreatedAt:   c.CreatedAt,
				CompletedOn: completionsBy[c.ID],
			})
		}
		export["commitments"] = out
	}

	checkInsBy := map[string][]exportedBehaviorCheckIn{}
	var checkIns []models.BehaviorCheckIn
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&checkIns).Error; err == nil {
		for _, c := range checkIns {
			checkInsBy[c.PlanID] = append(checkInsBy[c.PlanID], exportedBehaviorCheckIn{
				Status: c.Status, Note: c.Note, CreatedAt: c.CreatedAt,
			})
		}
	}

	var plans []models.BehaviorPlan
	if err := db.Where("user_id = ?", userID).Find(&plans).Error; err == nil && len(plans) > 0 {
		out := make([]exportedBehaviorPlan, 0, len(plans))
		for _, p := range plans {
			out = append(out, exportedBehaviorPlan{
				Status: p.Status, Behavior: p.Behavior, Identity: p.Identity, Motive: p.Motive,
				Trigger: p.Trigger, Context: p.Context, Frequency: p.Frequency, Days: p.Days,
				Obstacles: p.Obstacles, PlanB: p.PlanB, Area: p.Area, StartDate: p.StartDate,
				WinsCount: p.WinsCount, LastCheckInAt: p.LastCheckInAt, CreatedAt: p.CreatedAt,
				CheckIns: checkInsBy[p.ID],
			})
		}
		export["behaviorPlans"] = out
	}

	var reflections []models.IdentityReflection
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&reflections).Error; err == nil && len(reflections) > 0 {
		out := make([]exportedIdentityReflection, 0, len(reflections))
		for _, r := range reflections {
			out = append(out, exportedIdentityReflection{
				LearnedIdentity: r.LearnedIdentity, WhatItGave: r.WhatItGave,
				WhatItCosts: r.WhatItCosts, WhoBecoming: r.WhoBecoming,
				Qualities: r.Qualities, Evidence: r.Evidence, CreatedAt: r.CreatedAt,
			})
		}
		export["identityReflections"] = out
	}

	var acceptances []models.AcceptanceReflection
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&acceptances).Error; err == nil && len(acceptances) > 0 {
		out := make([]exportedAcceptanceReflection, 0, len(acceptances))
		for _, r := range acceptances {
			out = append(out, exportedAcceptanceReflection{
				Expected: r.Expected, Reality: r.Reality,
				CannotControl: r.CannotControl, CanInfluence: r.CanInfluence,
				ChooseToAccept: r.ChooseToAccept, WhereIAct: r.WhereIAct,
				NextStep: r.NextStep, CreatedAt: r.CreatedAt,
			})
		}
		export["acceptanceReflections"] = out
	}

	var sessions []models.CommunicationSession
	if err := db.Where("user_id = ?", userID).Order("start_time").Find(&sessions).Error; err == nil && len(sessions) > 0 {
		out := make([]exportedSession, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, exportedSession{
				StartTime: s.StartTime, EndTime: s.EndTime, Duration: s.Duration,
				Language: s.Language, SessionType: s.SessionType,
				UserEvaluation: s.UserEvaluation, UserFeedback: s.UserFeedback,
				UserSessionInsight: s.UserSessionInsight, SessionSummary: s.SessionSummary,
				Recap: s.Recap, RecapTitle: s.RecapTitle,
			})
		}
		export["communicationSessions"] = out
	}

	var messages []models.ChannelMessage
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&messages).Error; err == nil && len(messages) > 0 {
		out := make([]exportedChannelMessage, 0, len(messages))
		for _, m := range messages {
			out = append(out, exportedChannelMessage{
				Provider: m.Provider, Direction: m.Direction, Type: m.Type,
				Body: m.Body, CreatedAt: m.CreatedAt,
			})
		}
		export["channelMessages"] = out
	}

	var memories []models.UserMemory
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&memories).Error; err == nil && len(memories) > 0 {
		out := make([]exportedMemory, 0, len(memories))
		for _, m := range memories {
			out = append(out, exportedMemory{Category: m.Category, Content: m.Content, CreatedAt: m.CreatedAt})
		}
		export["memories"] = out
	}

	var wheels []models.WheelOfLifeExercise
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&wheels).Error; err == nil && len(wheels) > 0 {
		out := make([]exportedExercise, 0, len(wheels))
		for _, e := range wheels {
			out = append(out, exportedExercise{Data: e.Data, CreatedAt: e.CreatedAt})
		}
		export["wheelOfLifeExercises"] = out
	}

	var matrices []models.EisenhowerMatrixExercise
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&matrices).Error; err == nil && len(matrices) > 0 {
		out := make([]exportedExercise, 0, len(matrices))
		for _, e := range matrices {
			out = append(out, exportedExercise{Data: e.Data, CreatedAt: e.CreatedAt})
		}
		export["eisenhowerMatrixExercises"] = out
	}

	var ledger []models.BalanceTransaction
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&ledger).Error; err == nil && len(ledger) > 0 {
		out := make([]exportedTransaction, 0, len(ledger))
		for _, tx := range ledger {
			out = append(out, exportedTransaction{
				Type: string(tx.Type), AmountSeconds: tx.AmountSeconds, BalanceAfter: tx.BalanceAfter,
				SessionType: tx.SessionType, Product: tx.Product, Description: tx.Description,
				CreatedAt: tx.CreatedAt,
			})
		}
		export["balanceTransactions"] = out
	}

	var badges []models.UserBadge
	if err := db.Where("user_id = ?", userID).Order("earned_at").Find(&badges).Error; err == nil && len(badges) > 0 {
		out := make([]exportedBadge, 0, len(badges))
		for _, b := range badges {
			out = append(out, exportedBadge{Badge: b.BadgeType, EarnedAt: b.EarnedAt})
		}
		export["badges"] = out
	}

	attachmentsBy := map[string]int{}
	var attachments []models.FeedbackAttachment
	if err := db.Where("user_id = ?", userID).Find(&attachments).Error; err == nil {
		for _, a := range attachments {
			attachmentsBy[a.FeedbackID]++
		}
	}

	var feedback []models.Feedback
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&feedback).Error; err == nil && len(feedback) > 0 {
		out := make([]exportedFeedback, 0, len(feedback))
		for _, f := range feedback {
			out = append(out, exportedFeedback{
				Category: f.Category, Description: f.Description, Platform: f.Platform,
				AppVersion: f.AppVersion, Images: attachmentsBy[f.ID], CreatedAt: f.CreatedAt,
			})
		}
		export["feedback"] = out
	}

	var recommendations []models.Recommendation
	if err := db.Where("user_id = ?", userID).Order("created_at").Find(&recommendations).Error; err == nil && len(recommendations) > 0 {
		out := make([]exportedRecommendation, 0, len(recommendations))
		for _, rec := range recommendations {
			out = append(out, exportedRecommendation{
				Title: rec.Title, Type: rec.Type, Author: rec.Author,
				URL: rec.URL, Description: rec.Description, CreatedAt: rec.CreatedAt,
			})
		}
		export["recommendations"] = out
	}

	return export
}

// RequestUserDataExport implements api.ServerInterface: the asynchronous, emailed export.
//
// It answers 202 immediately and does the work in the background. A download is close to
// useless on mobile — the file lands somewhere the user cannot get at — whereas an email
// puts it where they can actually keep it. The trade is that the result arrives later, so
// the response says "on its way", never "here it is".
//
// Failures after the 202 are logged and not surfaced: there is no longer a request to fail.
// That is the honest consequence of accepting the job rather than doing it inline.
func (s *Server) RequestUserDataExport(w http.ResponseWriter, r *http.Request) {
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

	// Checked before accepting: an account with no address can never receive the export,
	// and 202-ing that is lying to the user.
	if user.Email == nil || *user.Email == "" {
		apierror.Write(w, http.StatusUnprocessableEntity, apierror.CodeInvalidPayload,
			"This account has no email address to send the export to.")
		return
	}

	email := *user.Email
	name := ""
	if user.Name != nil {
		name = *user.Name
	}
	locale := "en-US"
	if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
		locale = *user.PreferredLanguage
	}

	if notification.GlobalNotificationService == nil {
		s.logger.Error("data export requested with no notification service configured",
			zap.String("user_id", userID))
		apierror.Write(w, http.StatusServiceUnavailable, apierror.CodeInternalError,
			"Email delivery is not available right now. Please try again later.")
		return
	}

	// Captured now, not read inside the goroutine: the export runs after the response has
	// gone out, and reading the global at that point makes what it queries depend on
	// whatever else the process did in between.
	db := database.DB

	go func() {
		// This goroutine is detached from the request, so a panic in it would take the
		// whole process down rather than one response — it reads a dozen tables and
		// serializes whatever it finds.
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic while producing data export",
					zap.String("user_id", userID), zap.Any("panic", rec))
			}
		}()

		payload, err := json.Marshal(buildUserDataExport(db, userID))
		if err != nil {
			s.logger.Error("failed to build data export", zap.String("user_id", userID), zap.Error(err))
			return
		}
		if err := notification.GlobalNotificationService.SendDataExportEmail(email, name, locale, payload); err != nil {
			s.logger.Error("failed to email data export",
				zap.String("user_id", userID), zap.Int("bytes", len(payload)), zap.Error(err))
			return
		}
		s.logger.Info("data export emailed",
			zap.String("user_id", userID), zap.Int("bytes", len(payload)))
	}()

	w.WriteHeader(http.StatusAccepted)
}
