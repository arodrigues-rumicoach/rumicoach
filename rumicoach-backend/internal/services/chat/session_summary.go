package chat

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// Session-end synthesis screen (Filipa's "ecrã de síntese"): a reviewable snapshot the app
// renders when the session closes — the tangible sense of progress that carries the journey
// into the next session without explicitly selling it. Built deterministically from data
// already stored in the user's language during the session; the only fixed strings are the
// next-session questions, sent as localization KEYS so the client translates them (the
// backend never ships English text to the user).
//
// The payload is emitted as a `session_summary` WS message when terminate_session fires
// (the goodbye is still playing; the client shows the screen on session_terminated) and
// persisted on communication_sessions.session_summary so the app can re-render it any time.

// nextSessionQuestionKeys maps the session that just ended to the fixed question that
// seeds the next one, per Filipa's scripts.
// The onboarding intro is deliberately absent: it produces none of the anchors this
// screen is built from (vision, priority area, insight), so it has no synthesis screen —
// it hands straight over to Vision instead. Priorities closes the first pass of the
// journey: its successor is dynamic (the journey cycles to the least-recently-done deep
// session), so it has no fixed bridge question — its card simply omits next_session.
var nextSessionQuestionKeys = map[api.SessionType]map[string]string{
	api.SessionTypeSessionVision:     {"session_type": string(api.SessionTypeSessionMovement), "question_key": "next_question_movement_blockers"},
	api.SessionTypeSessionMovement:   {"session_type": string(api.SessionTypeSessionValues), "question_key": "next_question_values_why"},
	api.SessionTypeSessionValues:     {"session_type": string(api.SessionTypeSessionEnergy), "question_key": "next_question_energy_capacity"},
	api.SessionTypeSessionEnergy:     {"session_type": string(api.SessionTypeSessionDecisions), "question_key": "next_question_decisions_fears"},
	api.SessionTypeSessionDecisions:  {"session_type": string(api.SessionTypeSessionBeliefs), "question_key": "next_question_beliefs_stories"},
	api.SessionTypeSessionBeliefs:    {"session_type": string(api.SessionTypeSessionIdentity), "question_key": "next_question_identity_becoming"},
	api.SessionTypeSessionIdentity:   {"session_type": string(api.SessionTypeSessionAcceptance), "question_key": "next_question_acceptance_expectations"},
	api.SessionTypeSessionAcceptance: {"session_type": string(api.SessionTypeSessionPriorities), "question_key": "next_question_priorities_attention"},
}

// summaryCapableSessions is the set of session types that get a session-end synthesis
// screen at all. Every deep session in the journey has one (QA: Values ending with no
// card read as the session "not counting"); check-ins and the onboarding intro do not.
// Membership here is what decides whether a card exists — nextSessionQuestionKeys only
// decides whether the card carries a next-session bridge.
var summaryCapableSessions = map[api.SessionType]bool{
	api.SessionTypeSessionVision:     true,
	api.SessionTypeSessionMovement:   true,
	api.SessionTypeSessionValues:     true,
	api.SessionTypeSessionEnergy:     true,
	api.SessionTypeSessionDecisions:  true,
	api.SessionTypeSessionBeliefs:    true,
	api.SessionTypeSessionIdentity:   true,
	api.SessionTypeSessionAcceptance: true,
	api.SessionTypeSessionPriorities: true,
}

// buildSessionSummary assembles the synthesis payload for the session types that have a
// designed closing screen (every deep session). Returns nil when the session type has
// none or the essentials are missing.
//
// includeNextSession controls whether the "next session" bridge card is included. The
// panel reveals in two stages for a staged session (Vision): first WITHOUT next_session,
// as the AI starts its synthesis (the reflection is what's on screen and in its mouth at
// that point); then WITH next_session added, once the AI actually reaches the bridge
// sentence in the goodbye ("but one question still lingers..."). Showing the next-session
// preview before that sentence is spoken jumps ahead of the conversation (QA). A
// single-prompt session (Movement) has no staged reveal — its one call always passes true.
func (s *ChatSession) buildSessionSummary(includeNextSession bool) map[string]interface{} {
	if !summaryCapableSessions[s.SessionType] || s.User == nil {
		return nil
	}

	data := map[string]interface{}{
		"session_id":   s.SessionDB.ID,
		"session_type": string(s.SessionType),
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if next, ok := nextSessionQuestionKeys[s.SessionType]; ok && includeNextSession {
		data["next_session"] = next
	}

	// The vision and priority area give both screens their "why" anchor.
	if s.User.IdealLifeVision != nil && *s.User.IdealLifeVision != "" {
		data["vision"] = *s.User.IdealLifeVision
	}
	if area := s.loadPriorityAreaSummary(); area != nil {
		data["priority_area"] = area
	}

	// The user's own words: the insight they named at the closing of THIS session.
	var insights []models.UserMemory
	if err := database.DB.Where("user_id = ? AND category = ? AND created_at >= ?",
		s.UserID, "insight", s.SessionDB.StartTime).Order("created_at desc").Limit(1).
		Find(&insights).Error; err == nil && len(insights) > 0 {
		data["key_insight"] = insights[0].Content
	}

	// Commitments registered during the session — Vision's single first-step commitment
	// (save_vision_commitment, Step 4 of the closing) or Movement's board (the week's first
	// step and today's proof, from the structured-capture protocol). Deduplicated by
	// normalized title: the same user intention can reach the commitments table through
	// more than one tool (add_commitment + update_commitment_plan, or a behavior-plan
	// projection), and QA saw the same commitment rendered twice on the card.
	var tasks []models.Commitment
	if err := database.DB.Where("user_id = ? AND created_at >= ?",
		s.UserID, s.SessionDB.StartTime).Order("created_at asc").
		Find(&tasks).Error; err == nil && len(tasks) > 0 {
		commitments := make([]map[string]interface{}, 0, len(tasks))
		seen := make(map[string]bool, len(tasks))
		for _, t := range tasks {
			key := normalizeCommitmentTitle(t.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			c := map[string]interface{}{"title": t.Title, "type": t.Type}
			if t.Date != nil && *t.Date != "" {
				c["date"] = *t.Date
			}
			commitments = append(commitments, c)
		}
		data["commitments"] = commitments
	}

	// The Values session's anchor is the top values the user CHOSE — the clean list
	// written by save_top_values, never the free-text values memories (QA: the card
	// showed three long memory sentences instead of "Crescimento, Amor, Família").
	if s.SessionType == api.SessionTypeSessionValues && s.User != nil && len(s.User.TopValues) > 0 {
		data["values"] = []string(s.User.TopValues)
	}

	// The Movement session's subject is the blocker — Filipa's card spec opens with
	// "what has been blocking me". The newest obstacles memory captured during the
	// session carries it.
	if s.SessionType == api.SessionTypeSessionMovement {
		var obstacleMemories []models.UserMemory
		if err := database.DB.Where("user_id = ? AND category = ? AND created_at >= ?",
			s.UserID, "obstacles", s.SessionDB.StartTime).Order("created_at desc").Limit(1).
			Find(&obstacleMemories).Error; err == nil && len(obstacleMemories) > 0 {
			data["main_obstacle"] = obstacleMemories[0].Content
		}
	}

	// The Identity session's card is Filipa's Identity Reflection Card: the structured
	// synthesis captured by save_identity_reflection (who I learned to be / what it gave
	// me / what it costs / who I keep becoming / qualities / next evidence). Absent when
	// the model never confirmed a statement — the card then falls back to the standard
	// insight + commitments fields.
	if s.SessionType == api.SessionTypeSessionIdentity {
		var reflections []models.IdentityReflection
		if err := database.DB.Where("user_id = ? AND created_at >= ?",
			s.UserID, s.SessionDB.StartTime).Order("created_at desc").Limit(1).
			Find(&reflections).Error; err == nil && len(reflections) > 0 {
			r := reflections[0]
			ref := map[string]interface{}{
				"learned_identity": r.LearnedIdentity,
				"who_becoming":     r.WhoBecoming,
				"qualities":        []string(r.Qualities),
			}
			if r.WhatItGave != "" {
				ref["what_it_gave"] = r.WhatItGave
			}
			if r.WhatItCosts != "" {
				ref["what_it_costs"] = r.WhatItCosts
			}
			if r.Evidence != "" {
				ref["evidence"] = r.Evidence
			}
			data["identity_reflection"] = ref
		}
	}

	// The Acceptance session's card is Filipa's Acceptance Reflection Card: the
	// structured synthesis captured by save_acceptance_reflection (what I expected /
	// what is true / what I can't control / what I can influence / what I accept /
	// where I act / next step). Absent when the model never confirmed a reflection —
	// the card then falls back to the standard insight + commitments fields.
	if s.SessionType == api.SessionTypeSessionAcceptance {
		var reflections []models.AcceptanceReflection
		if err := database.DB.Where("user_id = ? AND created_at >= ?",
			s.UserID, s.SessionDB.StartTime).Order("created_at desc").Limit(1).
			Find(&reflections).Error; err == nil && len(reflections) > 0 {
			r := reflections[0]
			ref := map[string]interface{}{
				"expected": r.Expected,
				"reality":  r.Reality,
			}
			if r.CannotControl != "" {
				ref["cannot_control"] = r.CannotControl
			}
			if r.CanInfluence != "" {
				ref["can_influence"] = r.CanInfluence
			}
			if r.ChooseToAccept != "" {
				ref["choose_to_accept"] = r.ChooseToAccept
			}
			if r.WhereIAct != "" {
				ref["where_i_act"] = r.WhereIAct
			}
			if r.NextStep != "" {
				ref["next_step"] = r.NextStep
			}
			data["acceptance_reflection"] = ref
		}
	}

	if s.SessionType == api.SessionTypeSessionMovement {
		var plans []models.BehaviorPlan
		if err := database.DB.Where("user_id = ? AND created_at >= ?", s.UserID, s.SessionDB.StartTime).
			Order("created_at desc").Limit(1).Find(&plans).Error; err == nil && len(plans) > 0 {
			data["behavior_plan"] = map[string]interface{}{
				"behavior": plans[0].Behavior,
				"identity": plans[0].Identity,
			}
		}
	}

	return data
}

// loadPriorityAreaSummary resolves the user's focus area with its current wheel
// score and the user's reasoning, all stored in the user's language.
func (s *ChatSession) loadPriorityAreaSummary() map[string]interface{} {
	if s.User == nil || s.User.FocusArea == nil || *s.User.FocusArea == "" {
		return nil
	}
	focusArea := *s.User.FocusArea
	area := map[string]interface{}{
		"name": focusArea,
	}

	var exercises []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).
		Find(&exercises).Error; err == nil && len(exercises) > 0 && exercises[0].Data != "" {
		var items []WheelOfLifeItem
		if err := json.Unmarshal([]byte(exercises[0].Data), &items); err == nil {
			for _, item := range items {
				if item.Name == focusArea {
					area["score"] = int(item.CurrentScore)
					area["max_score"] = 10
					if item.Reasoning != "" && item.Reasoning != "Initial setup" {
						area["reasoning"] = item.Reasoning
					}
					break
				}
			}
		}
	}
	return area
}

// normalizeCommitmentTitle reduces a commitment title to a comparison key: lowercase,
// whitespace collapsed, trailing punctuation dropped. Used to spot the same user
// intention saved twice through different tools — an exact-string match misses
// "Caminhar 10 minutos." vs "caminhar 10 minutos".
func normalizeCommitmentTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	t = strings.TrimRight(t, ".!…")
	return strings.Join(strings.Fields(t), " ")
}

// emitSessionTasksPanel pushes the commitments registered DURING this session to the
// client as a data-bearing panel message (`session_tasks_update`) — the in-session
// "interactive board" of Filipa's session 2 script. Like the Wheel of Life, the panel is
// opened by the tool that carries its data (add_commitment / behavior-plan projection), never
// by navigation: the user stays inside the session and watches their commitment appear.
func (s *ChatSession) emitSessionTasksPanel() {
	if s.SessionDB.StartTime.IsZero() {
		return
	}
	var tasks []models.Commitment
	if err := database.DB.Where("user_id = ? AND created_at >= ?",
		s.UserID, s.SessionDB.StartTime).Order("created_at asc").
		Find(&tasks).Error; err != nil || len(tasks) == 0 {
		return
	}
	items := make([]map[string]interface{}, 0, len(tasks))
	seen := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		key := normalizeCommitmentTitle(t.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		item := map[string]interface{}{"id": t.ID, "title": t.Title, "type": t.Type, "done": t.Done}
		if t.Date != nil && *t.Date != "" {
			item["date"] = *t.Date
		}
		items = append(items, item)
	}
	s.writeClientJSON(map[string]interface{}{
		"type": "session_tasks_update",
		"data": map[string]interface{}{"tasks": items},
	})
}

// emitAndPersistSessionSummary sends the synthesis screen payload to the client and stores
// it on the session row for later re-rendering. Failures only log — the summary must never
// break session termination.
//
// Idempotent per stage, independently: the first call with includeNextSession=false (the
// synthesis reveal) and the first call with includeNextSession=true (the next-session
// addition) each fire at most once. This lets a staged session (Vision) emit twice — once
// early via the ◆▦ marker, once later via ◆▧ — while a single-prompt session (Movement)
// makes one call with true and gets the complete payload in one shot. A defensive guard
// also refuses a false call once true has already fired, so a stray early-stage retry can
// never downgrade a panel that already has its next-session card.
func (s *ChatSession) emitAndPersistSessionSummary(includeNextSession bool) {
	s.geminiMutex.Lock()
	var alreadyDone bool
	if includeNextSession {
		alreadyDone = s.sessionSummaryNextSessionSent
		s.sessionSummaryNextSessionSent = true
	} else {
		alreadyDone = s.sessionSummaryEmitted || s.sessionSummaryNextSessionSent
		s.sessionSummaryEmitted = true
	}
	s.geminiMutex.Unlock()
	if alreadyDone {
		return
	}

	data := s.buildSessionSummary(includeNextSession)
	if data == nil {
		return
	}

	s.writeClientJSON(map[string]interface{}{
		"type": "session_summary",
		"data": data,
	})

	raw, err := json.Marshal(data)
	if err != nil {
		s.logger.Warn("Failed to marshal session summary", zap.Error(err))
		return
	}
	if err := database.DB.Model(&models.CommunicationSession{}).Where("id = ?", s.SessionDB.ID).
		Update("session_summary", string(raw)).Error; err != nil {
		s.logger.Warn("Failed to persist session summary", zap.Error(err))
		return
	}
	s.logger.Info("Session summary emitted and persisted", zap.String("session_type", string(s.SessionType)))
}
