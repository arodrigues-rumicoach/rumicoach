package companion

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// The companion can now act on commitments, not just read them. Everything a user says
// naturally by message — "already did the walk", "add taking my meds every morning",
// "I'm done with journaling" — used to be met with warm words and no record, which is
// worse than saying nothing: the user believes it was logged.
//
// These write through the SAME models and rules as the app and the live sessions
// (CommitmentCompletion for a recurring day, Done for a one-time), so the three surfaces
// can never disagree about what the user has done.

// commitmentToolDeclarations are appended to the companion's function declarations.
var commitmentToolDeclarations = []map[string]any{
	{
		"name":        "complete_commitment",
		"description": "Mark one of the user's commitments as done, or undo that, for TODAY. Call it when they tell you they did something they had committed to (or that they had not, after all). Identify the commitment by its title as it appears in the user's commitments in your context — you do not need an exact match, close is enough. Only call this for something they actually said they did; never assume, and never call it to 'encourage' them.",
		"parameters": map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "STRING",
					"description": "The commitment's title, as shown in your context.",
				},
				"done": map[string]any{
					"type":        "BOOLEAN",
					"description": "true when they did it, false to undo a completion they say was a mistake. Defaults to true.",
				},
			},
			"required": []string{"title"},
		},
	},
	{
		"name":        "add_commitment",
		"description": "Add a new commitment the user explicitly asks to track. Only call this when they ASK for it — a passing wish ('I should really sleep more') is not a request. Prefer suggesting a full coaching session in the app when the topic deserves real depth; this is for the small, concrete things people add between sessions.",
		"parameters": map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "STRING",
					"description": "Short, concrete, in the user's language — what they will actually do (e.g. 'Walk 10 minutes after lunch').",
				},
				"type": map[string]any{
					"type":        "STRING",
					"description": "'recurring' for a habit on set weekdays, 'one_time' for something with a single date.",
				},
				"days": map[string]any{
					"type":        "ARRAY",
					"items":       map[string]any{"type": "INTEGER"},
					"description": "For recurring: the weekdays, 1 = Monday ... 7 = Sunday.",
				},
				"date": map[string]any{
					"type":        "STRING",
					"description": "For one_time: the date, YYYY-MM-DD.",
				},
				"end_date": map[string]any{
					"type":        "STRING",
					"description": "For recurring: the day the habit stops, YYYY-MM-DD. ALWAYS agree one with the user (typically two to four weeks out). A habit with no end runs forever and becomes something they are failing rather than finishing.",
				},
			},
			"required": []string{"title", "type"},
		},
	},
	{
		"name":        "end_commitment",
		"description": "End a recurring commitment when the user says they are done with it or no longer want it. Everything they already did towards it stays in their history — this is finishing a habit, not erasing it. Identify it by title as shown in your context. Never call this because they missed a few days; only when they actually ask to stop.",
		"parameters": map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "STRING",
					"description": "The commitment's title, as shown in your context.",
				},
			},
			"required": []string{"title"},
		},
	},
}

// resolveCommitment finds the commitment a user meant from the title the model passed.
// People say "the walk", not a UUID, so matching is forgiving: exact (case-insensitive)
// first, then substring either way. An ambiguous match is an error listing the
// candidates rather than a guess — acting on the wrong commitment is worse than asking.
func resolveCommitment(userID, title string) (*models.Commitment, error) {
	needle := strings.ToLower(strings.TrimSpace(title))
	if needle == "" {
		return nil, fmt.Errorf("no title given")
	}

	var all []models.Commitment
	if err := database.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&all).Error; err != nil {
		return nil, fmt.Errorf("could not read the commitments")
	}
	// An ended commitment is not a candidate: acting on one the user finished weeks ago
	// is never what they meant.
	var live []models.Commitment
	for _, c := range all {
		if c.EndedAt == nil || c.EndedAt.After(time.Now()) {
			live = append(live, c)
		}
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("the user has no active commitments")
	}

	var exact, partial []models.Commitment
	for _, c := range live {
		lc := strings.ToLower(c.Title)
		switch {
		case lc == needle:
			exact = append(exact, c)
		case strings.Contains(lc, needle) || strings.Contains(needle, lc):
			partial = append(partial, c)
		}
	}
	candidates := exact
	if len(candidates) == 0 {
		candidates = partial
	}

	switch len(candidates) {
	case 1:
		c := candidates[0]
		return &c, nil
	case 0:
		titles := make([]string, 0, len(live))
		for _, c := range live {
			titles = append(titles, c.Title)
		}
		sort.Strings(titles)
		return nil, fmt.Errorf("no commitment matches %q. The user's commitments are: %s", title, strings.Join(titles, "; "))
	default:
		titles := make([]string, 0, len(candidates))
		for _, c := range candidates {
			titles = append(titles, c.Title)
		}
		sort.Strings(titles)
		return nil, fmt.Errorf("%q matches more than one commitment (%s) — ask the user which they mean", title, strings.Join(titles, "; "))
	}
}

// completeCommitment marks today's occurrence done or not done, using exactly the rules
// the app uses: a per-day row for recurring habits, the master flag for one-time ones.
func (s *Service) completeCommitment(userID string, args map[string]any) map[string]any {
	title, _ := args["title"].(string)
	done := true
	if v, ok := args["done"].(bool); ok {
		done = v
	}

	c, err := resolveCommitment(userID, title)
	if err != nil {
		return map[string]any{"status": "error", "message": err.Error()}
	}

	// No HTTP request here, so no timezone header — days are UTC. The app remains the
	// source of truth on screen; a few hours of drift at the day boundary is acceptable
	// for a chat that says "nice, logged it".
	today := time.Now().UTC().Format("2006-01-02")

	if c.Type == "recurring" {
		if done {
			completion := models.CommitmentCompletion{UserID: userID, CommitmentID: c.ID, Date: today}
			if err := database.DB.Where("user_id = ? AND commitment_id = ? AND date = ?", userID, c.ID, today).
				FirstOrCreate(&completion).Error; err != nil {
				s.logger.Error("companion: failed to record completion", zap.Error(err))
				return map[string]any{"status": "error", "message": "could not save that"}
			}
		} else if err := database.DB.Where("user_id = ? AND commitment_id = ? AND date = ?", userID, c.ID, today).
			Delete(&models.CommitmentCompletion{}).Error; err != nil {
			s.logger.Error("companion: failed to remove completion", zap.Error(err))
			return map[string]any{"status": "error", "message": "could not save that"}
		}
	} else if err := database.DB.Model(&models.Commitment{}).
		Where("id = ? AND user_id = ?", c.ID, userID).Update("done", done).Error; err != nil {
		s.logger.Error("companion: failed to update commitment", zap.Error(err))
		return map[string]any{"status": "error", "message": "could not save that"}
	}

	s.logger.Info("companion: commitment completion updated",
		zap.String("userID", userID), zap.String("commitment", c.Title), zap.Bool("done", done))
	verb := "marked as done for today"
	if !done {
		verb = "marked as not done"
	}
	return map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("%q %s. Acknowledge it warmly and briefly — do not list their other commitments back at them.", c.Title, verb),
	}
}

// addCommitment creates a commitment the user asked for, applying the same horizon rule
// the coaching sessions use: a recurring habit without an end runs forever.
func (s *Service) addCommitment(userID string, args map[string]any) map[string]any {
	title, _ := args["title"].(string)
	title = strings.TrimSpace(title)
	kind, _ := args["type"].(string)
	kind = strings.TrimSpace(strings.ToLower(kind))

	if title == "" {
		return map[string]any{"status": "error", "message": "title is required"}
	}
	if kind != "recurring" && kind != "one_time" {
		return map[string]any{"status": "error", "message": "type must be 'recurring' or 'one_time'"}
	}

	c := models.Commitment{
		UserID: userID,
		Origin: models.CommitmentOriginManual,
		Title:  title,
		Type:   kind,
	}

	switch kind {
	case "recurring":
		days := toIntSlice(args["days"])
		if len(days) == 0 {
			return map[string]any{"status": "error", "message": "a recurring commitment needs the weekdays — ask the user which days"}
		}
		c.Days = models.IntSlice(days)
		if raw, ok := args["end_date"].(string); ok && strings.TrimSpace(raw) != "" {
			if end, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil && end.After(time.Now()) {
				c.EndedAt = &end
			}
		}
	case "one_time":
		raw, _ := args["date"].(string)
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err != nil {
			return map[string]any{"status": "error", "message": "a one-time commitment needs a date in YYYY-MM-DD — ask the user when"}
		}
		d := strings.TrimSpace(raw)
		c.Date = &d
	}

	if err := database.DB.Create(&c).Error; err != nil {
		s.logger.Error("companion: failed to add commitment", zap.Error(err))
		return map[string]any{"status": "error", "message": "could not save that"}
	}

	s.logger.Info("companion: commitment added", zap.String("userID", userID), zap.String("title", title))
	msg := fmt.Sprintf("%q added. Confirm it back in one short sentence.", title)
	if kind == "recurring" && c.EndedAt == nil {
		msg = fmt.Sprintf("%q added, but with no end date — ask the user how long they want to keep it, then set it.", title)
	}
	return map[string]any{"status": "success", "message": msg}
}

// endCommitment finishes a recurring habit, preserving everything already done.
func (s *Service) endCommitment(userID string, args map[string]any) map[string]any {
	title, _ := args["title"].(string)

	c, err := resolveCommitment(userID, title)
	if err != nil {
		return map[string]any{"status": "error", "message": err.Error()}
	}
	if c.Type != "recurring" {
		return map[string]any{"status": "error", "message": "only recurring commitments can be ended; a one-time one simply passes"}
	}

	now := time.Now()
	if err := database.DB.Model(&models.Commitment{}).
		Where("id = ? AND user_id = ?", c.ID, userID).Update("ended_at", now).Error; err != nil {
		s.logger.Error("companion: failed to end commitment", zap.Error(err))
		return map[string]any{"status": "error", "message": "could not save that"}
	}

	s.logger.Info("companion: commitment ended", zap.String("userID", userID), zap.String("commitment", c.Title))
	return map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("%q is finished. Everything they already did towards it stays in their history — close it warmly, as a completed chapter rather than something abandoned.", c.Title),
	}
}

// toIntSlice reads the weekday array, which JSON decoding hands over as float64s.
func toIntSlice(v any) []int {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		switch n := item.(type) {
		case float64:
			out = append(out, int(n))
		case int:
			out = append(out, n)
		}
	}
	return out
}
