package handlers

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/services/usage"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// GetUsage implements api.ServerInterface.
//
// Deprecated: the Subscription & usage screen renders the statement view
// (GetTransactions with grouped=true) since Aug 2026. This endpoint serves
// only app versions already in the field — delete it, the usage service and
// their tests once the statement-based release is the minimum supported app
// version.
//
// Returns the caller's usage history: billed sessions and per-day
// companion-message groups, newest first, plus lifetime totals. The
// X-Timezone header decides which calendar day a message falls on, same as
// the journey gates.
func (s *Server) GetUsage(w http.ResponseWriter, r *http.Request, params api.GetUsageParams) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok || userID == "" {
		apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Unauthorized")
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
	if limit > 100 {
		limit = 100
	}

	entries, totals, err := usage.History(ctx, userID, GetTimezoneLocation(r))
	if err != nil {
		s.logger.Error("failed to build usage history", zap.String("user_id", userID), zap.Error(err))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Failed to load usage history")
		return
	}

	total := len(entries)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	apiItems := make([]api.UsageEntry, 0, end-start)
	for _, e := range entries[start:end] {
		apiItems = append(apiItems, usageEntryToAPI(e))
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp := api.UsageHistoryResponse{
		Items: &apiItems,
		Totals: &api.UsageTotals{
			TotalSeconds:       &totals.TotalSeconds,
			SessionSeconds:     &totals.SessionSeconds,
			FreeSessionSeconds: &totals.FreeSessionSeconds,
			SessionCount:       &totals.SessionCount,
			MessageSeconds:     &totals.MessageSeconds,
			MessageCount:       &totals.MessageCount,
		},
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   &total,
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func usageEntryToAPI(e usage.Entry) api.UsageEntry {
	item := api.UsageEntry{
		Kind:       api.UsageEntryKind(e.Kind),
		OccurredAt: e.OccurredAt,
		Seconds:    e.Seconds,
	}
	switch e.Kind {
	case "session":
		item.SessionId = e.SessionID
		item.SessionType = e.SessionType
		free := e.Free
		item.Free = &free
	case "messages":
		day := e.Day
		count := e.MessageCount
		item.Date = &day
		item.MessageCount = &count
	}
	return item
}
