// Package checkin implements the recurring daily check-in coaching session (formerly
// "daily_checkin") as a session.Session implementation. It adapts its opening to the user's
// session frequency and acts as a gateway that can offer the deep session planned for today.
package checkin

import (
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

// checkinTools are the task tools available across the check-in sessions: close the session
// plus the Eisenhower Matrix exercise the daily check-in can offer when the user feels
// overwhelmed. (Common tools like save_memory are added by the chat runtime.)
//
// complete_current_task is deliberately NOT here: a check-in is a single state with no next
// stage (NextOnCompleteTask returns false), so every call it could ever make fails. Declaring
// it invited exactly that — the model reached for it to "close" the check-in and got two
// consecutive errors instead (QA). A check-in ends through terminate_session, or simply by
// the user leaving. Declared tools must always have a scripted trigger, and vice versa.
var checkinTools = []string{
	"init_eisenhower_matrix",
	"update_eisenhower_matrix",
	"delete_eisenhower_matrix_tasks",
	"add_commitment",
	"remove_commitment",
	"save_behavior_plan",
	"log_behavior_checkin",
	"terminate_session",
	"show_screen",
	"save_session_insight",
}

// --- Daily check-in ---

type Daily struct{}

func NewDaily() *Daily { return &Daily{} }

func (Daily) Name() string                      { return "checkin" }
func (Daily) Type() api.SessionType             { return api.SessionTypeCheckin }
func (Daily) InitialState() models.SessionState { return models.StateCheckin }

func (Daily) SystemPersona(v session.Voice) string { return session.DefaultPersona(v) }

// Instructions builds the daily prompt dynamically from the user's session-frequency pattern.
// When a deep/weekly/monthly session is planned for today, the check-in first offers it (the
// gateway); if the user would rather talk, it falls through to the regular check-in.
func (Daily) Instructions(_ models.SessionState, ctx session.Context) string {
	base := buildDailyCheckInInstructions(analyzeSessionFrequency(ctx.UserID))
	if ctx.PlannedSession != "" && ctx.PlannedSession != api.SessionTypeCheckin {
		return buildPlannedSessionGateway(session.DisplayName(ctx.PlannedSession)) + base + session.BehaviorChangeProtocol
	}
	return base + session.BehaviorChangeProtocol
}

// dailyCheckinTools adds the gateway tool (start_planned_session) to the shared check-in tools.
var dailyCheckinTools = append([]string{"start_planned_session"}, checkinTools...)

func (Daily) ToolNames(models.SessionState) []string { return dailyCheckinTools }

func (Daily) NextOnCompleteTask(models.SessionState) (session.Transition, bool) {
	return session.Transition{}, false
}

func (Daily) NeedsRestart(_, _ models.SessionState) bool { return false }

func (Daily) ReviewPrompt() string { return session.DefaultReviewPrompt }
