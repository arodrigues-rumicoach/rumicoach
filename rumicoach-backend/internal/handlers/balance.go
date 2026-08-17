package handlers

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GetTransactions implements api.ServerInterface.
// Returns the caller's balance ledger, newest first. With grouped=true, runs of
// same-day message debits collapse into one row each (the statement view the
// Subscription & usage screen renders); the X-Timezone header decides the day
// boundary, same as /me/usage.
func (s *Server) GetTransactions(w http.ResponseWriter, r *http.Request, params api.GetTransactionsParams) {
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

	var apiItems []api.BalanceTransaction
	var total int64
	if params.Grouped != nil && *params.Grouped {
		// Folding decides where page boundaries fall, so pagination happens on
		// the folded slice, not in SQL — same trade as GetUsage.
		entries, err := balance.Statement(ctx, userID, GetTimezoneLocation(r))
		if err != nil {
			s.logger.Error("failed to build balance statement", zap.String("user_id", userID), zap.Error(err))
			apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Failed to list transactions")
			return
		}
		total = int64(len(entries))
		start := (page - 1) * limit
		if start > len(entries) {
			start = len(entries)
		}
		end := start + limit
		if end > len(entries) {
			end = len(entries)
		}
		apiItems = make([]api.BalanceTransaction, 0, end-start)
		for _, e := range entries[start:end] {
			apiTx := balanceTransactionToAPI(e.BalanceTransaction)
			if e.MessageCount > 0 {
				day := e.Day
				count := e.MessageCount
				apiTx.Day = &day
				apiTx.MessageCount = &count
			}
			apiItems = append(apiItems, apiTx)
		}
	} else {
		items, rawTotal, err := balance.ListTransactions(ctx, userID, page, limit)
		if err != nil {
			s.logger.Error("failed to list balance transactions", zap.String("user_id", userID), zap.Error(err))
			apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Failed to list transactions")
			return
		}
		total = rawTotal
		apiItems = make([]api.BalanceTransaction, len(items))
		for i, tx := range items {
			apiItems[i] = balanceTransactionToAPI(tx)
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp := api.BalanceTransactionPaginatedResponse{
		Items: &apiItems,
		Pagination: &api.PaginationInfo{
			CurrentPage:  &page,
			ItemsPerPage: &limit,
			TotalItems:   func(i int) *int { return &i }(int(total)),
			TotalPages:   &totalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// PostAdminUsersIdCredits implements api.ServerInterface.
// Manually credits purchased minutes to a user — the stand-in for the payment
// provider until subscriptions/top-ups are integrated.
func (s *Server) PostAdminUsersIdCredits(w http.ResponseWriter, r *http.Request, id string) {
	var req api.AdminCreditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request body")
		return
	}

	if !req.Type.Valid() {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "type must be one of: subscription, top_up")
		return
	}

	var amountSeconds int64
	switch {
	case req.AmountSeconds != nil && req.AmountMinutes != nil:
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Provide exactly one of amountSeconds or amountMinutes")
		return
	case req.AmountSeconds != nil:
		amountSeconds = *req.AmountSeconds
	case req.AmountMinutes != nil:
		amountSeconds = int64(*req.AmountMinutes) * 60
	default:
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Provide exactly one of amountSeconds or amountMinutes")
		return
	}
	if amountSeconds <= 0 {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Credit amount must be positive")
		return
	}

	entry, err := balance.Credit(r.Context(), id, amountSeconds, models.BalanceTransactionType(req.Type), req.Product, req.Description)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierror.Write(w, http.StatusNotFound, apierror.CodeAccountNotFound, "User not found")
			return
		}
		s.logger.Error("failed to credit balance", zap.String("user_id", id), zap.Error(err))
		apierror.Write(w, http.StatusInternalServerError, apierror.CodeInternalError, "Failed to credit balance")
		return
	}

	resp := balanceTransactionToAPI(*entry)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func balanceTransactionToAPI(tx models.BalanceTransaction) api.BalanceTransaction {
	txType := api.BalanceTransactionType(tx.Type)
	createdAt := tx.CreatedAt

	apiTx := api.BalanceTransaction{
		Id:            &tx.ID,
		Type:          &txType,
		AmountSeconds: &tx.AmountSeconds,
		BalanceAfter:  &tx.BalanceAfter,
		SessionId:     tx.SessionID,
		SessionType:   tx.SessionType,
		Product:       tx.Product,
		Description:   tx.Description,
		CreatedAt:     &createdAt,
	}
	if tx.Type == models.BalanceTxSubscription {
		apiTx.Plan = subscriptionPlan(tx.Product)
	}

	return apiTx
}

// subscriptionPlan reads the billing cadence out of a subscription credit's
// product id, so clients translate an enum instead of each parsing store slugs
// their own way. The ledger stores whatever id the store used — App Store
// Connect ids, web-billing slugs — and all of them name their cadence. Annual
// is checked first: "year"/"annual" never appear in a monthly id, while
// "month" could appear in a "12-month" annual one. Nil when the id names
// neither; clients fall back to a generic label.
func subscriptionPlan(product *string) *api.BalanceTransactionPlan {
	if product == nil {
		return nil
	}
	id := strings.ToLower(*product)
	var plan api.BalanceTransactionPlan
	switch {
	case strings.Contains(id, "annual") || strings.Contains(id, "year"):
		plan = api.Annual
	case strings.Contains(id, "month"):
		plan = api.Monthly
	default:
		return nil
	}
	return &plan
}
