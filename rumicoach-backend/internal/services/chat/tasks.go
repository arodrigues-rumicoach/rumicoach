package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
	"github.com/rumi/rumi-be/internal/services/chat/session/vision"
	"github.com/rumi/rumi-be/internal/services/journey"
	"go.uber.org/zap"
)

// WheelOfLifeItem represents a single Wheel of Life category entry.
type WheelOfLifeItem struct {
	Name         string  `json:"name"`
	CurrentScore float64 `json:"currentScore"`
	Reasoning    string  `json:"reasoning"`
}

// EisenhowerMatrixItem represents a single task in the Eisenhower Matrix.
type EisenhowerMatrixItem struct {
	Task      string `json:"task"`
	Quadrant  string `json:"quadrant"` // urgent_important, not_urgent_important, urgent_not_important, not_urgent_not_important
	Reasoning string `json:"reasoning"`
}

// EisenhowerMatrixData represents the four quadrants of the matrix.
type EisenhowerMatrixData struct {
	UrgentImportant       []EisenhowerMatrixItem `json:"urgent_important"`
	NotUrgentImportant    []EisenhowerMatrixItem `json:"not_urgent_important"`
	UrgentNotImportant    []EisenhowerMatrixItem `json:"urgent_not_important"`
	NotUrgentNotImportant []EisenhowerMatrixItem `json:"not_urgent_not_important"`
}

const emotionalClosingInstructions = `[TASK INSTRUCTION]
### EMOTIONAL CLOSING
**Objective:** End with identity, hope, and reassurance.

**PHASE 1: THE EMOTIONAL CLOSING SCRIPT**
1. Deliver a powerful, emotional closing message using this script as a guide (ensure a soft, slow, and comforting tone):
   "Keep this goal with you. Transformation rarely happens through major moments. It happens through small choices repeated over the days. Today, you don't need to change your entire life. You only need to take the next step.
   The person you imagined at the beginning of this session is not someone distant that you have to struggle to 'build'. It is someone who already lives inside you, and who is revealed whenever you choose to act in alignment with what you value."
2. **STOP AND WAIT.** Let the user absorb this and respond.

**PHASE 2: TRANSITION**
3. Once the user replies, call the complete_current_task tool (with 'current_state' set to "EMOTIONAL_CLOSING") to move to the final session ending phase.`

const endingSessionInstructions = `[TASK INSTRUCTION]
### SESSION ENDING
**Objective:** Final goodbye and termination.

**PHASE 1: THE UNIVERSAL CLOSING**
1. Deliver the final goodbye message, adapting it naturally to the user:
   "I am here to walk with you on this journey. Until our next session."
2. **CALL THE TOOL (CRITICAL DIRECTIVE):** In the SAME TURN as your final goodbye, you MUST call the 'terminate_session' tool natively. You are strictly forbidden from ending the response without calling the tool.`

// GetNextInstructions builds the next instructions based on the user's state
func (s *ChatSession) GetNextInstructions(user *models.User) string {
	name := "there"
	if user != nil && user.Name != nil && *user.Name != "" {
		names := strings.Fields(*user.Name)
		if len(names) > 0 {
			name = names[0]
		}
	}

	state := s.CurrentState()

	userID := ""
	if user != nil {
		userID = user.ID
	}
	ctx := session.Context{
		UserID:    userID,
		FirstName: name,
		Wheel:     s.loadOnboardingWheelItems(user),
	}
	// The daily check-in acts as a gateway: if a deep/weekly/monthly session is planned for
	// today and not yet done, surface it so the check-in can offer to start it.
	if s.SessionType == api.SessionTypeCheckin {
		ctx.PlannedSession = journey.PlannedSessionForToday(s.UserID, s.Location)
	}

	// Registered per-session flows (prompts, tools, transitions) live in their own packages
	// under session/ and are reached through the registry. Delegate first.
	if sess, ok := sessions.Get(s.SessionType); ok {
		// The wheel is populated live during StateVisionWheelOfLife via the tool calls
		// themselves (set_wheel_of_life_categories / update_wheel_of_life), so the client
		// already has it in memory there. But GetNextInstructions also runs whenever a
		// connection is (re-)established — a fresh reconnect after the app was closed, or
		// a resume straight into the Metaphor phase — and a client that starts cold has
		// nothing to render until something sends it the wheel again. The Metaphor script
		// opens by asking the user to "look at your Wheel of Life", so resuming there with
		// an empty screen breaks the exercise (QA: resumed into Metaphor, AI continued
		// talking, wheel never appeared).
		if state == models.StateVisionWheelOfLife || state == models.StateVisionMetaphor {
			s.SyncWheelVisuals()
		}
		if instr := sess.Instructions(state, ctx); instr != "" {
			return instr
		}
	}

	switch state {
	case models.StateEmotionalClosing:
		return emotionalClosingInstructions

	case models.StateEndingSession:
		return endingSessionInstructions

	default:
		// Fallback: the daily check-in adapts to any returning user. Reaching it for a
		// non-checkin session type means the session doesn't recognize the user's state
		// (this silently masked new users with the legacy 'ONBOARDING' state being
		// greeted with the post-onboarding daily-coaching script) — make it loud.
		if s.SessionType != api.SessionTypeCheckin {
			s.logger.Warn("Session type has no instructions for user state; falling back to the daily check-in prompt",
				zap.String("session_type", string(s.SessionType)),
				zap.String("state", string(state)))
		}
		if sess, ok := sessions.Get(api.SessionTypeCheckin); ok {
			return sess.Instructions(state, ctx)
		}
		return ""
	}
}

// loadOnboardingWheelItems loads the user's latest Wheel of Life data and converts it
// into the shape the onboarding package consumes.
func (s *ChatSession) loadOnboardingWheelItems(user *models.User) []vision.WheelItem {
	if user == nil {
		return nil
	}
	var exercise models.WheelOfLifeExercise
	database.DB.Where("user_id = ?", user.ID).Order("created_at desc").Limit(1).Find(&exercise)

	var wheelData []WheelOfLifeItem
	if exercise.Data != "" {
		json.Unmarshal([]byte(exercise.Data), &wheelData) //nolint:errcheck
	}

	items := make([]vision.WheelItem, len(wheelData))
	for i, w := range wheelData {
		items[i] = vision.WheelItem{Name: w.Name, CurrentScore: w.CurrentScore, Reasoning: w.Reasoning}
	}
	return items
}

func (s *ChatSession) getNextPendingCategory(user *models.User) string {
	if user == nil {
		return ""
	}
	var exercise models.WheelOfLifeExercise
	database.DB.Where("user_id = ?", user.ID).Order("created_at desc").Limit(1).Find(&exercise)

	var wheelData []WheelOfLifeItem
	if exercise.Data != "" {
		json.Unmarshal([]byte(exercise.Data), &wheelData) //nolint:errcheck
	}

	for _, item := range wheelData {
		if item.CurrentScore <= 0 {
			return item.Name
		}
	}
	return ""
}

// GetDynamicTransitionPrompt builds the transition cue injected on TURN_COMPLETE after a
// wheel tool call. wheelIntroSkipped marks a setup turn where the model created the areas
// without speaking the introduction first (see WheelDynamicTransition).
func (s *ChatSession) GetDynamicTransitionPrompt(user *models.User, wheelIntroSkipped bool) string {
	state := s.CurrentState()

	if state == models.StateVisionWheelOfLife {
		return vision.WheelDynamicTransition(s.loadOnboardingWheelItems(user), wheelIntroSkipped)
	}

	return "[SYSTEM: The session state has advanced. Please begin the next phase naturally according to your active task instructions.]"
}

// handleStartPlannedSession switches the live session over to the session the user just
// agreed to begin: the Vision session when the onboarding intro hands over, or the deep
// session planned for today when a daily check-in offers it. It re-resolves that session
// authoritatively server-side (never from anything the model passed in), points the
// session at it, and triggers a Gemini connection restart so the new session's system
// prompt and tools are loaded; the restart directive then begins it.
func (s *ChatSession) handleStartPlannedSession(args map[string]interface{}) (string, error) {
	planned := s.resolvePlannedHandover()
	if planned == "" {
		return `{"status": "error", "message": "There is no planned session to start right now. Continue the current conversation normally."}`, nil
	}
	name := session.DisplayName(planned)

	// A multi-state session (Vision) builds its Section 9 from users.state. Handed over
	// from a state it does not own — the check-in gateway hands over from CHECKIN — the
	// instruction builder falls back to the check-in prompt, so the restarted connection
	// carried checkin instructions with a "begin your Vision session" directive; the model
	// got contradictory orders and the session died (QA). Move the state to the planned
	// session's opening state, but only when the current state is foreign to it: a user
	// resuming mid-flow (e.g. parked at VISION_WHEEL_OF_LIFE) must NOT be reset to the
	// start. Single-prompt sessions ignore state entirely, so their handover is untouched.
	if sess, ok := sessions.Get(planned); ok && s.User != nil && s.User.State != nil {
		if sess.Instructions(models.SessionState(*s.User.State), session.Context{}) == "" {
			initial := string(sess.InitialState())
			if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
				Update("state", initial).Error; err != nil {
				s.logger.Error("Failed to move user state for planned-session handover", zap.Error(err))
			} else {
				s.User.State = &initial
				s.logger.Info("[State change] planned-session handover",
					zap.String("planned_session", string(planned)), zap.String("new_state", initial))
			}
		}
	}

	s.geminiMutex.Lock()
	s.SessionType = planned
	s.pendingRestart = true
	// The handover is seamless from the user's side — they were speaking with you a
	// moment ago — so the new session must NOT open with a fresh greeting. Sessions
	// whose script has its own welcome (Vision) are told to skip it here.
	s.restartInstructions = fmt.Sprintf("[SYSTEM: The user has just agreed to begin their %s session, in the conversation you are already having with them. You greeted them moments ago, so do NOT greet them again, do NOT re-introduce yourself, and do NOT deliver any welcome script your task instructions describe for a fresh conversation. Continue straight into the first substantive step of your CURRENT ACTIVE TASK INSTRUCTIONS (Section 9 of your system instructions). Do NOT mention pausing, restarting, switching, or the previous conversation.]", name)
	s.geminiMutex.Unlock()

	// The handover starts a NEW session from the user's (and the journey gates')
	// perspective, so it gets its own communication_sessions row: close the current
	// one and open a row typed as the planned session.
	s.rolloverSessionDB()

	// The intro's mini tour leaves the user on the memories or Journey screen (screen id
	// "growth"); the exercise
	// they just agreed to happens on the session screen, so put them back there before the
	// restart delivers its first line. Harmless when they are already on it.
	s.writeClientJSON(map[string]interface{}{
		"type": "show_screen",
		"data": map[string]string{"screen": "session"},
	})

	// The conversation screen labels itself with the session name (QA: users lose their
	// "mental GPS" without it); the handover changes what session is being held, so tell
	// the client the new type the same way session_created announced the first one.
	s.writeClientJSON(map[string]interface{}{
		"type": "session_type_update",
		"data": map[string]string{"session_type": string(planned)},
	})

	s.logger.Info("Session gateway: switching to planned session",
		zap.String("from", string(s.SessionType)), zap.String("planned_session", string(planned)))
	return `{"status": "success", "message": "Starting the planned session now. Stop generating and yield immediately."}`, nil
}

// resolvePlannedHandover returns the session the current one hands over to. The onboarding
// intro always hands over to Vision — Vision is the rest of what used to be one long
// onboarding and unlocks with no wait, so it is not gated behind today's DailyJourney
// snapshot (which, during the intro, still names onboarding itself).
func (s *ChatSession) resolvePlannedHandover() api.SessionType {
	if s.SessionType == api.SessionTypeOnboarding {
		return api.SessionTypeSessionVision
	}
	return journey.PlannedSessionForToday(s.UserID, s.Location)
}
