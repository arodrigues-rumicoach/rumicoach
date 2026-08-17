package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// Behavior Change Protocol tool handlers (see docs: Filipa's cross-session skill).
// Design invariants enforced HERE, not just in prompts (repo practice):
//   - a plan is only created once the protocol actually happened: behavior + identity +
//     trigger are required, so the model cannot save a vague intention as a plan;
//   - at most models.MaxActiveBehaviorPlans active plans — focus beats volume;
//   - the plan projects into ONE recurring Task (origin=behavior) that rides the existing
//     DailyJourney tracking; the plan itself has no checkboxes;
//   - WinsCount only goes up; a miss is stored for the coach and never echoed as failure.

var validBehaviorPlanStatuses = map[string]bool{
	models.BehaviorPlanStatusActive:    true,
	models.BehaviorPlanStatusParked:    true,
	models.BehaviorPlanStatusGraduated: true,
	models.BehaviorPlanStatusArchived:  true,
}

// findBehaviorPlan matches a plan by behavior text: exact case-insensitive first, then
// containment either way (the model often paraphrases slightly). Prefers active plans.
func findBehaviorPlan(plans []models.BehaviorPlan, behavior string) *models.BehaviorPlan {
	b := strings.ToLower(strings.TrimSpace(behavior))
	var fallback *models.BehaviorPlan
	for i := range plans {
		p := strings.ToLower(strings.TrimSpace(plans[i].Behavior))
		exact := p == b
		loose := strings.Contains(p, b) || strings.Contains(b, p)
		if exact && plans[i].Status == models.BehaviorPlanStatusActive {
			return &plans[i]
		}
		if (exact || loose) && fallback == nil {
			fallback = &plans[i]
		}
	}
	return fallback
}

func (s *ChatSession) handleSaveBehaviorPlan(args map[string]interface{}) (string, error) {
	behavior, _ := args["behavior"].(string)
	behavior = strings.TrimSpace(behavior)
	if behavior == "" {
		return `{"status": "error", "message": "missing 'behavior' parameter"}`, nil
	}

	var plans []models.BehaviorPlan
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at asc").Find(&plans).Error; err != nil {
		return "", fmt.Errorf("failed to load behavior plans: %w", err)
	}

	identity := strings.TrimSpace(getStringArg(args, "identity"))
	motive := strings.TrimSpace(getStringArg(args, "motive"))
	trigger := strings.TrimSpace(getStringArg(args, "trigger"))
	contextStr := strings.TrimSpace(getStringArg(args, "context"))
	frequency := strings.TrimSpace(getStringArg(args, "frequency"))
	obstacles := strings.TrimSpace(getStringArg(args, "obstacles"))
	planB := strings.TrimSpace(getStringArg(args, "plan_b"))
	area := strings.TrimSpace(getStringArg(args, "area"))
	status := strings.ToLower(strings.TrimSpace(getStringArg(args, "status")))

	if status != "" && !validBehaviorPlanStatuses[status] {
		return `{"status": "error", "message": "invalid 'status': use 'active', 'parked', 'graduated', or 'archived'"}`, nil
	}

	var days models.IntSlice
	if daysStr := strings.TrimSpace(getStringArg(args, "days")); daysStr != "" {
		if err := json.Unmarshal([]byte(daysStr), &days); err != nil {
			return `{"status": "error", "message": "invalid 'days': pass a JSON array of ISO weekdays, e.g. '[1,3,5]'"}`, nil
		}
		for _, d := range days {
			if d < 1 || d > 7 {
				return `{"status": "error", "message": "invalid 'days': weekdays are 1 (Monday) through 7 (Sunday)"}`, nil
			}
		}
	}

	existing := findBehaviorPlan(plans, behavior)

	if existing == nil {
		// Creation guard: the plan is the OUTPUT of the protocol, not a note. Without the
		// identity and the trigger the protocol did not happen — send the model back to it.
		var missing []string
		if identity == "" {
			missing = append(missing, "'identity' (the person this behavior makes them — co-create it with the user)")
		}
		if trigger == "" {
			missing = append(missing, "'trigger' (the existing routine it anchors to: after what?)")
		}
		if len(missing) > 0 {
			return fmt.Sprintf(`{"status": "error", "message": "REJECTED: a behavior plan is the outcome of the Behavior Change Protocol, not a quick note. Missing: %s. Walk those steps with the user first, then call save_behavior_plan again with everything filled in."}`, strings.Join(missing, " and ")), nil
		}

		// Focus cap: more active plans means shallower follow-ups on all of them.
		if status == "" || status == models.BehaviorPlanStatusActive {
			var activeNames []string
			for _, p := range plans {
				if p.Status == models.BehaviorPlanStatusActive {
					activeNames = append(activeNames, p.Behavior)
				}
			}
			if len(activeNames) >= models.MaxActiveBehaviorPlans {
				return fmt.Sprintf(`{"status": "error", "message": "REJECTED: the user already has %d active behavior commitments (%s) — the maximum for real focus. Do NOT add another silently: reflect this back to the user warmly and let THEM choose — deepen or adjust an existing commitment, park one to make room, or keep this new intention for later. Then call save_behavior_plan again accordingly."}`, len(activeNames), strings.Join(activeNames, "; ")), nil
			}
		}

		today := time.Now().Format("2006-01-02")
		plan := models.BehaviorPlan{
			UserID:    s.UserID,
			Status:    models.BehaviorPlanStatusActive,
			Behavior:  behavior,
			Identity:  identity,
			Motive:    motive,
			Trigger:   trigger,
			Context:   contextStr,
			Frequency: frequency,
			Days:      days,
			Obstacles: obstacles,
			PlanB:     planB,
			StartDate: &today,
		}
		if status != "" {
			plan.Status = status
		}
		if area != "" {
			plan.Area = &area
		}
		if err := database.DB.Create(&plan).Error; err != nil {
			return "", fmt.Errorf("failed to save behavior plan: %w", err)
		}
		s.syncBehaviorPlanTask(&plan)
		s.logger.Info("Behavior plan created", zap.String("plan_id", plan.ID), zap.String("behavior", behavior))
		return `{"status": "success", "message": "Behavior plan saved. Close this step per your task instructions — and remember: this is about who the user is becoming, not about the task itself."}`, nil
	}

	// Update path: only overwrite fields the model actually provided.
	if identity != "" {
		existing.Identity = identity
	}
	if motive != "" {
		existing.Motive = motive
	}
	if trigger != "" {
		existing.Trigger = trigger
	}
	if contextStr != "" {
		existing.Context = contextStr
	}
	if frequency != "" {
		existing.Frequency = frequency
	}
	if len(days) > 0 {
		existing.Days = days
	}
	if obstacles != "" {
		existing.Obstacles = obstacles
	}
	if planB != "" {
		existing.PlanB = planB
	}
	if area != "" {
		existing.Area = &area
	}
	if status != "" {
		existing.Status = status
	}
	// The behavior wording itself may have been refined (simplified after a check-in).
	existing.Behavior = behavior

	if err := database.DB.Save(existing).Error; err != nil {
		return "", fmt.Errorf("failed to update behavior plan: %w", err)
	}
	s.syncBehaviorPlanTask(existing)
	s.logger.Info("Behavior plan updated", zap.String("plan_id", existing.ID), zap.String("behavior", behavior), zap.String("status", existing.Status))

	switch existing.Status {
	case models.BehaviorPlanStatusGraduated:
		return `{"status": "success", "message": "Plan updated to graduated. Celebrate the IDENTITY with the user — this behavior is part of who they are now — and stop following it up in future sessions."}`, nil
	case models.BehaviorPlanStatusParked:
		return `{"status": "success", "message": "Plan parked. Reassure the user this stays safe with you for when the moment is right — never frame it as giving up or failing."}`, nil
	default:
		return `{"status": "success", "message": "Behavior plan updated."}`, nil
	}
}

// syncBehaviorPlanTask keeps the plan's recurring Commitment projection in step with the plan:
// active plans with weekdays get (or update) a recurring commitment; plans leaving the active
// state have their projection removed. The commitment is system-owned (origin=behavior) — all
// the coaching data lives on the plan, so deleting the projection loses nothing.
func (s *ChatSession) syncBehaviorPlanTask(plan *models.BehaviorPlan) {
	if plan.Status == models.BehaviorPlanStatusActive && len(plan.Days) > 0 {
		if plan.TaskID != nil {
			if err := database.DB.Model(&models.Commitment{}).Where("id = ? AND user_id = ?", *plan.TaskID, plan.UserID).
				Updates(map[string]interface{}{"title": plan.Behavior, "days": plan.Days}).Error; err != nil {
				s.logger.Warn("Failed to update behavior plan task", zap.Error(err))
			}
			return
		}
		task := models.Commitment{
			UserID: plan.UserID,
			Origin: models.CommitmentOriginBehavior,
			Title:  plan.Behavior,
			Type:   "recurring",
			Days:   plan.Days,
		}
		if err := database.DB.Create(&task).Error; err != nil {
			s.logger.Warn("Failed to create behavior plan task", zap.Error(err))
			return
		}
		plan.TaskID = &task.ID
		if err := database.DB.Model(&models.BehaviorPlan{}).Where("id = ?", plan.ID).Update("task_id", task.ID).Error; err != nil {
			s.logger.Warn("Failed to link behavior plan task", zap.Error(err))
		}
		s.SyncDailyJourneyCommitments()
		s.emitSessionTasksPanel()
		return
	}

	if plan.TaskID != nil {
		if err := database.DB.Where("id = ? AND user_id = ? AND origin = ?", *plan.TaskID, plan.UserID, models.CommitmentOriginBehavior).
			Delete(&models.Commitment{}).Error; err != nil {
			s.logger.Warn("Failed to remove behavior plan task", zap.Error(err))
			return
		}
		plan.TaskID = nil
		if err := database.DB.Model(&models.BehaviorPlan{}).Where("id = ?", plan.ID).Update("task_id", nil).Error; err != nil {
			s.logger.Warn("Failed to unlink behavior plan task", zap.Error(err))
		}
		s.SyncDailyJourneyCommitments()
	}
}

func (s *ChatSession) handleLogBehaviorCheckin(args map[string]interface{}) (string, error) {
	behavior := strings.TrimSpace(getStringArg(args, "behavior"))
	status := strings.ToLower(strings.TrimSpace(getStringArg(args, "status")))
	note := strings.TrimSpace(getStringArg(args, "note"))

	if behavior == "" || status == "" {
		return `{"status": "error", "message": "missing 'behavior' or 'status' parameter"}`, nil
	}
	if status != models.BehaviorCheckInKept && status != models.BehaviorCheckInPartial && status != models.BehaviorCheckInMissed {
		return `{"status": "error", "message": "invalid 'status': use 'kept', 'partial', or 'missed'"}`, nil
	}

	var plans []models.BehaviorPlan
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at asc").Find(&plans).Error; err != nil {
		return "", fmt.Errorf("failed to load behavior plans: %w", err)
	}
	plan := findBehaviorPlan(plans, behavior)
	if plan == nil {
		names := make([]string, 0, len(plans))
		for _, p := range plans {
			names = append(names, p.Behavior)
		}
		if len(names) == 0 {
			return `{"status": "error", "message": "The user has no behavior plans yet. If they are committing to a new behavior, walk the Behavior Change Protocol and call save_behavior_plan instead."}`, nil
		}
		return fmt.Sprintf(`{"status": "error", "message": "No behavior plan matches '%s'. The user's plans are: %s. Call log_behavior_checkin again with the 'behavior' copied from this list."}`, behavior, strings.Join(names, "; ")), nil
	}

	checkIn := models.BehaviorCheckIn{
		PlanID: plan.ID,
		UserID: s.UserID,
		Status: status,
		Note:   note,
	}
	if err := database.DB.Create(&checkIn).Error; err != nil {
		return "", fmt.Errorf("failed to save behavior check-in: %w", err)
	}

	updates := map[string]interface{}{"last_check_in_at": time.Now()}
	// Plan B executed counts as a win by design — the fallback IS part of the plan. Wins
	// only accumulate; misses never subtract anything.
	if status == models.BehaviorCheckInKept || status == models.BehaviorCheckInPartial {
		updates["wins_count"] = plan.WinsCount + 1
	}
	if err := database.DB.Model(&models.BehaviorPlan{}).Where("id = ?", plan.ID).Updates(updates).Error; err != nil {
		s.logger.Warn("Failed to update behavior plan after check-in", zap.Error(err))
	}
	s.logger.Info("Behavior check-in logged", zap.String("plan_id", plan.ID), zap.String("status", status))

	switch status {
	case models.BehaviorCheckInMissed:
		return `{"status": "success", "message": "Check-in recorded as information, not failure. Never judge: ask what made it harder than expected, then adjust the plan together (usually by SIMPLIFYING the behavior — never by demanding more motivation). If you adjust it, call save_behavior_plan with the changes. Protect the identity, not the behavior."}`, nil
	case models.BehaviorCheckInPartial:
		return `{"status": "success", "message": "Check-in recorded — and using the fallback still counts as showing up. Celebrate that they honored the commitment in a hard moment (identity, not execution), then explore lightly what got in the way of the full version."}`, nil
	default:
		return `{"status": "success", "message": "Win recorded. Celebrate the IDENTITY, not just the execution — ask what helped, and reflect back who they are becoming by repeating this."}`, nil
	}
}

// getStringArg reads an optional string argument, tolerating absence.
func getStringArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

// LoadBehaviorPlansContext loads the user's non-archived behavior plans plus each plan's
// most recent check-in, for rendering into the system prompt (FormatBehaviorPlansContext).
func LoadBehaviorPlansContext(userID string, logger *zap.Logger) ([]models.BehaviorPlan, map[string]models.BehaviorCheckIn) {
	if database.DB == nil {
		return nil, nil
	}
	var plans []models.BehaviorPlan
	if err := database.DB.Where("user_id = ? AND status IN ?", userID,
		[]string{models.BehaviorPlanStatusActive, models.BehaviorPlanStatusParked}).
		Order("created_at asc").Find(&plans).Error; err != nil {
		logger.Warn("Failed to load behavior plans", zap.Error(err))
		return nil, nil
	}
	lastCheckIns := make(map[string]models.BehaviorCheckIn)
	for _, p := range plans {
		var ci []models.BehaviorCheckIn
		if err := database.DB.Where("plan_id = ?", p.ID).Order("created_at desc").Limit(1).Find(&ci).Error; err == nil && len(ci) > 0 {
			lastCheckIns[p.ID] = ci[0]
		}
	}
	return plans, lastCheckIns
}
