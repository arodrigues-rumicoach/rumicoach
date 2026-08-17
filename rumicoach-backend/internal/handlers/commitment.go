package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// calculateCommitmentStatus computes a commitment's display status relative to todayStr ("YYYY-MM-DD").
func calculateCommitmentStatus(done bool, commitmentType string, date *string, todayStr string) api.CommitmentStatus {
	if done {
		return api.Completed
	}
	if commitmentType == "one_time" && date != nil && *date != "" && *date < todayStr {
		return api.Overdue
	}
	return api.Pending
}

// toAPICommitment converts a master models.Commitment into an api.Commitment, computing the display
// status relative to todayStr ("YYYY-MM-DD"). done is the completion flag to reflect (the master
// Commitment.Done for the commitments view, or today's snapshot done for the daily view).
func toAPICommitment(t models.Commitment, done bool, todayStr string) api.Commitment {
	var daysPtr *[]int
	if len(t.Days) > 0 {
		d := []int(t.Days)
		daysPtr = &d
	}
	return api.Commitment{
		Id:      t.ID,
		Title:   t.Title,
		Type:    api.CommitmentType(t.Type),
		Origin:  api.CommitmentOrigin(t.Origin),
		Days:    daysPtr,
		Date:    t.Date,
		EndedAt: t.EndedAt,
		Status:  calculateCommitmentStatus(done, t.Type, t.Date, todayStr),
	}
}

// nextOccurrence resolves the next date (today included) a commitment happens on, in
// "YYYY-MM-DD". For one-time commitments it is simply their date — which may be in the
// past (overdue). For recurring commitments it is the next matching ISO weekday. Returns
// "" when the commitment has no resolvable occurrence (recurring with no days, one-time
// with no date).
func nextOccurrence(t models.Commitment, nowLocal time.Time) string {
	switch t.Type {
	case "one_time":
		if t.Date == nil || *t.Date == "" {
			return ""
		}
		return *t.Date
	case "recurring":
		if len(t.Days) == 0 {
			return ""
		}
		for offset := 0; offset < 7; offset++ {
			day := nowLocal.AddDate(0, 0, offset)
			isoWeekday := int(day.Weekday())
			if isoWeekday == 0 {
				isoWeekday = 7
			}
			for _, d := range t.Days {
				if d == isoWeekday {
					return day.Format("2006-01-02")
				}
			}
		}
	}
	return ""
}

// GetCommitments implements api.ServerInterface — lists the user's present and future
// commitments, one entry per commitment, ordered by its next occurrence: overdue one-time
// commitments first (oldest date first), then today's, then upcoming. One-time commitments
// completed on a past date are over and excluded. done/status for commitments due today
// come from today's DailyJourney snapshot (recurring reset each day); future occurrences
// are always pending.
func (s *Server) GetCommitments(w http.ResponseWriter, r *http.Request, params api.GetCommitmentsParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	loc := GetTimezoneLocation(r)
	nowLocal := time.Now().In(loc)
	todayStr := nowLocal.Format("2006-01-02")

	history := params.History != nil && *params.History

	var commitments []models.Commitment
	order := "created_at asc"
	if history {
		order = "created_at desc"
	}
	if err := database.DB.Where("user_id = ?", userID).Order(order).Find(&commitments).Error; err != nil {
		s.logger.Error("Failed to fetch commitments", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch commitments"}`, http.StatusInternalServerError)
		return
	}

	// Today's completion state for recurring commitments lives in CommitmentCompletion.
	var completions []models.CommitmentCompletion
	doneToday := make(map[string]bool)
	if err := database.DB.Where("user_id = ? AND date = ?", userID, todayStr).Find(&completions).Error; err == nil {
		for _, comp := range completions {
			doneToday[comp.CommitmentID] = true
		}
	}

	// History answers a different question from the list ahead: not "what is next for
	// this commitment?" but "what did the user actually do, and what should they have
	// done?". That is per OCCURRENCE, not per commitment — a recurring commitment has
	// one row per scheduled day it has lived through, which is exactly what the old
	// history mode was missing (it returned a single row pointing at a FUTURE date).
	if history {
		s.writeCommitmentHistory(w, userID, commitments, nowLocal)
		return
	}

	type upcoming struct {
		commitment api.Commitment
		nextDate   string
		createdAt  time.Time
	}
	list := make([]upcoming, 0, len(commitments))
	for _, t := range commitments {
		next := nextOccurrence(t, nowLocal)
		if next == "" {
			continue
		}
		// A recurring commitment runs until its end date. An end in the PAST means it is
		// over and belongs to history alone; an end in the FUTURE is a planned horizon
		// ("every morning for the next 30 days") and the commitment is still live — it
		// just stops owing days once the next occurrence would fall beyond the end.
		if t.EndedAt != nil && next > t.EndedAt.In(nowLocal.Location()).Format("2006-01-02") {
			continue
		}
		// A one-time commitment completed on a past date is over — it has no present
		// or future occurrence. (Overdue-but-pending ones stay, at the top.)
		// If history=true, we keep them all.
		if !history && t.Type == "one_time" && t.Done && next < todayStr {
			continue
		}

		// For one-time commitments, completion is global regardless of date.
		// For recurring commitments, we only reflect today's completion if due today; future occurrences are pending.
		done := false
		if t.Type == "one_time" {
			done = t.Done
		} else if next <= todayStr {
			done = doneToday[t.ID]
		}

		c := toAPICommitment(t, done, todayStr)
		nd := next
		c.NextDate = &nd
		list = append(list, upcoming{commitment: c, nextDate: next, createdAt: t.CreatedAt})
	}

	sort.SliceStable(list, func(i, j int) bool {
		if list[i].nextDate != list[j].nextDate {
			return list[i].nextDate < list[j].nextDate
		}
		return list[i].createdAt.Before(list[j].createdAt)
	})

	apiCommitments := make([]api.Commitment, 0, len(list))
	for _, u := range list {
		apiCommitments = append(apiCommitments, u.commitment)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiCommitments)
}

// CreateCommitment implements api.ServerInterface — creates a manual commitment (origin = manual).
func (s *Server) CreateCommitment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req api.CreateCommitmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Title == "" || (req.Type != api.CreateCommitmentRequestTypeOneTime && req.Type != api.CreateCommitmentRequestTypeRecurring) {
		http.Error(w, `{"error": "title and a valid type (one_time|recurring) are required"}`, http.StatusBadRequest)
		return
	}

	commitment := models.Commitment{
		UserID: userID,
		Origin: models.CommitmentOriginManual,
		Title:  req.Title,
		Type:   string(req.Type),
		Date:   req.Date,
	}
	if req.Days != nil {
		commitment.Days = models.IntSlice(*req.Days)
	}

	if err := database.DB.Create(&commitment).Error; err != nil {
		s.logger.Error("Failed to create commitment", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to create commitment"}`, http.StatusInternalServerError)
		return
	}

	loc := GetTimezoneLocation(r)
	todayStr := time.Now().In(loc).Format("2006-01-02")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toAPICommitment(commitment, commitment.Done, todayStr))
}

// UpdateCommitment implements api.ServerInterface — partial update of a commitment (edit fields / toggle done).
func (s *Server) UpdateCommitment(w http.ResponseWriter, r *http.Request, commitmentId string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req api.UpdateCommitmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	var commitment models.Commitment
	if err := database.DB.Where("id = ? AND user_id = ?", commitmentId, userID).First(&commitment).Error; err != nil {
		http.Error(w, `{"error": "Commitment not found"}`, http.StatusNotFound)
		return
	}

	loc := GetTimezoneLocation(r)
	todayStr := time.Now().In(loc).Format("2006-01-02")

	if req.Title != nil {
		commitment.Title = *req.Title
	}
	if req.Type != nil {
		commitment.Type = string(*req.Type)
	}
	if req.Days != nil {
		commitment.Days = models.IntSlice(*req.Days)
	}
	if req.Date != nil {
		commitment.Date = req.Date
	}
	// endDate is tri-state: absent leaves it alone, a date sets the horizon, and an
	// explicit empty string clears it so the habit runs on.
	if req.EndDate != nil {
		if *req.EndDate == "" {
			commitment.EndedAt = nil
		} else if end, err := time.Parse("2006-01-02", *req.EndDate); err == nil {
			commitment.EndedAt = &end
		}
	}

	finalDone := commitment.Done
	if req.Done != nil {
		commitment.Done = *req.Done
		finalDone = *req.Done

		// Handle daily completion for recurring commitments
		if commitment.Type == "recurring" {
			if *req.Done {
				// Mark as done for today
				completion := models.CommitmentCompletion{
					UserID:       userID,
					CommitmentID: commitmentId,
					Date:         todayStr,
				}
				// Use FirstOrCreate to avoid duplicates
				if err := database.DB.Where("user_id = ? AND commitment_id = ? AND date = ?", userID, commitmentId, todayStr).FirstOrCreate(&completion).Error; err != nil {
					s.logger.Error("Failed to record commitment completion", zap.Error(err), zap.String("commitment_id", commitmentId))
				}
			} else {
				// Mark as not done (delete completion record for today)
				if err := database.DB.Where("user_id = ? AND commitment_id = ? AND date = ?", userID, commitmentId, todayStr).Delete(&models.CommitmentCompletion{}).Error; err != nil {
					s.logger.Error("Failed to delete commitment completion", zap.Error(err), zap.String("commitment_id", commitmentId))
				}
			}
		}
	}

	if err := database.DB.Save(&commitment).Error; err != nil {
		s.logger.Error("Failed to update commitment", zap.Error(err), zap.String("commitment_id", commitmentId))
		http.Error(w, `{"error": "Failed to update commitment"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toAPICommitment(commitment, finalDone, todayStr))
}

// DeleteCommitment implements api.ServerInterface — deletes a commitment the user owns.
func (s *Server) DeleteCommitment(w http.ResponseWriter, r *http.Request, commitmentId string) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var commitment models.Commitment
	if err := database.DB.Where("id = ? AND user_id = ?", commitmentId, userID).First(&commitment).Error; err != nil {
		http.Error(w, `{"error": "Commitment not found"}`, http.StatusNotFound)
		return
	}

	// Ending a recurring commitment is not the same as erasing it. The user is saying
	// "I am done with this habit", and everything they already did towards it is part of
	// their history — deleting the row would silently take those weeks away from the
	// history screen. So it stops generating occurrences from today and the record stays.
	// One-time commitments carry no history beyond themselves and are removed outright.
	if commitment.Type == "recurring" && commitment.EndedAt == nil {
		now := time.Now()
		if err := database.DB.Model(&models.Commitment{}).Where("id = ? AND user_id = ?", commitmentId, userID).
			Update("ended_at", now).Error; err != nil {
			s.logger.Error("Failed to end commitment", zap.Error(err), zap.String("commitment_id", commitmentId))
			http.Error(w, `{"error": "Failed to delete commitment"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	res := database.DB.Where("id = ? AND user_id = ?", commitmentId, userID).Delete(&models.Commitment{})
	if res.Error != nil {
		s.logger.Error("Failed to delete commitment", zap.Error(res.Error), zap.String("commitment_id", commitmentId))
		http.Error(w, `{"error": "Failed to delete commitment"}`, http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, `{"error": "Commitment not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// historyMaxDays bounds how far back the history reaches. A recurring commitment with no
// end runs forever, so without a floor the response would keep growing for the life of
// the account. A year is far more than any screen scrolls through.
const historyMaxDays = 365

// writeCommitmentHistory renders every PAST occurrence of the user's commitments: the ones
// they did and the ones they should have done. One entry per occurrence, newest first.
//
//   - one_time: a single occurrence on its date — completed if done, missed if the date
//     has passed without it.
//   - recurring: one entry per scheduled weekday between the day it was created and the
//     day it ended (or today), each completed or missed according to the per-day record.
//
// Occurrences are bounded on both sides by real events — creation and end/today — so an
// endless habit cannot produce an endless list; historyMaxDays is the final backstop.
func (s *Server) writeCommitmentHistory(w http.ResponseWriter, userID string, commitments []models.Commitment, nowLocal time.Time) {
	loc := nowLocal.Location()
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	todayStr := today.Format("2006-01-02")
	floor := today.AddDate(0, 0, -historyMaxDays)

	// Every per-day completion in range, keyed by commitment and date.
	var completions []models.CommitmentCompletion
	if err := database.DB.
		Where("user_id = ? AND date >= ?", userID, floor.Format("2006-01-02")).
		Find(&completions).Error; err != nil {
		s.logger.Error("Failed to fetch commitment completions", zap.Error(err), zap.String("user_id", userID))
		http.Error(w, `{"error": "Failed to fetch commitments"}`, http.StatusInternalServerError)
		return
	}
	doneOn := make(map[string]map[string]bool, len(completions))
	for _, c := range completions {
		if doneOn[c.CommitmentID] == nil {
			doneOn[c.CommitmentID] = map[string]bool{}
		}
		doneOn[c.CommitmentID][c.Date] = true
	}

	entries := make([]api.Commitment, 0, len(commitments))

	for _, t := range commitments {
		switch t.Type {
		case "one_time":
			if t.Date == nil || *t.Date == "" || *t.Date > todayStr {
				continue // no date, or still in the future: not history yet
			}
			status := api.Missed
			if t.Done {
				status = api.Completed
			}
			entries = append(entries, historyEntry(t, *t.Date, status))

		case "recurring":
			if len(t.Days) == 0 {
				continue
			}
			days := make(map[int]bool, len(t.Days))
			for _, d := range t.Days {
				days[d] = true
			}

			// Walk from the day it was created (never before) to the day it ended
			// (or today) — the occurrences that actually existed.
			start := startOfLocalDay(t.CreatedAt, loc)
			if start.Before(floor) {
				start = floor
			}
			end := today
			if t.EndedAt != nil {
				ended := startOfLocalDay(*t.EndedAt, loc)
				// The end day itself no longer counts as owed.
				if ended.Before(end) {
					end = ended.AddDate(0, 0, -1)
				}
			}

			for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
				iso := int(d.Weekday())
				if iso == 0 {
					iso = 7
				}
				if !days[iso] {
					continue
				}
				dateStr := d.Format("2006-01-02")
				status := api.Missed
				if doneOn[t.ID][dateStr] {
					status = api.Completed
				}
				entries = append(entries, historyEntry(t, dateStr, status))
			}
		}
	}

	// Newest first; ties broken by title so the order is stable across requests.
	sort.SliceStable(entries, func(i, j int) bool {
		di, dj := "", ""
		if entries[i].OccurrenceDate != nil {
			di = *entries[i].OccurrenceDate
		}
		if entries[j].OccurrenceDate != nil {
			dj = *entries[j].OccurrenceDate
		}
		if di != dj {
			return di > dj
		}
		return entries[i].Title < entries[j].Title
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// historyEntry builds one past-occurrence entry. nextDate is deliberately left unset:
// in history the meaningful date is the occurrence itself, and carrying a future date
// alongside a past row is what made the old response so confusing to render.
func historyEntry(t models.Commitment, date string, status api.CommitmentStatus) api.Commitment {
	var daysPtr *[]int
	if len(t.Days) > 0 {
		d := []int(t.Days)
		daysPtr = &d
	}
	occurrence := date
	return api.Commitment{
		Id:             t.ID,
		Title:          t.Title,
		Type:           api.CommitmentType(t.Type),
		Origin:         api.CommitmentOrigin(t.Origin),
		Days:           daysPtr,
		Date:           t.Date,
		OccurrenceDate: &occurrence,
		EndedAt:        t.EndedAt,
		Status:         status,
	}
}

// startOfLocalDay returns local midnight of t's calendar day in loc.
func startOfLocalDay(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	return time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, loc)
}
