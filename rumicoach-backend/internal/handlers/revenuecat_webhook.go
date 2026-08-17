package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// revenueCatEnvelope is the webhook body RevenueCat delivers: a single event
// wrapped with the API version.
type revenueCatEnvelope struct {
	APIVersion string          `json:"api_version"`
	Event      revenueCatEvent `json:"event"`
}

type revenueCatEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// AppUserID is whatever the mobile client passed to Purchases.logIn — our
	// backend user id. Aliases carries every id RevenueCat has merged for this
	// subscriber, including "$RCAnonymousID:..." placeholders from before login.
	AppUserID         string   `json:"app_user_id"`
	OriginalAppUserID string   `json:"original_app_user_id"`
	Aliases           []string `json:"aliases"`
	ProductID         string   `json:"product_id"`
	PeriodType        string   `json:"period_type"` // TRIAL, INTRO, NORMAL, PROMOTIONAL
	Store             string   `json:"store"`       // APP_STORE, PLAY_STORE, STRIPE, ...
	Environment       string   `json:"environment"` // SANDBOX or PRODUCTION
	TransactionID     string   `json:"transaction_id"`
	// CancelReason qualifies CANCELLATION events. CUSTOMER_SUPPORT is the one that
	// means money went back (there is no dedicated REFUND event type); UNSUBSCRIBE
	// and the rest are auto-renew changes that leave granted minutes alone.
	CancelReason  string   `json:"cancel_reason"`
	Price         *float64 `json:"price"`
	Currency      string   `json:"currency"`
	PurchasedAtMs int64    `json:"purchased_at_ms"`
}

// revenueCatCreditTypes are the event types that represent money charged for a
// product, and the ledger type each maps to. Everything else (cancellations,
// expirations, billing issues, transfers, product changes) is acked and logged:
// the minutes bank keeps already-granted minutes, so those events carry no
// balance action today.
var revenueCatCreditTypes = map[string]models.BalanceTransactionType{
	"INITIAL_PURCHASE":      models.BalanceTxSubscription,
	"RENEWAL":               models.BalanceTxSubscription,
	"NON_RENEWING_PURCHASE": models.BalanceTxTopUp, // consumable top-ups
}

// PostWebhooksRevenuecat implements api.ServerInterface. RevenueCat delivers
// purchase events here; purchases and renewals credit the minutes bank through
// balance.CreditPurchase, keyed by the event id so redeliveries can never
// credit twice. Like the other webhooks the route is outside the authenticated
// prefixes — authenticity comes from the exact Authorization header value
// configured on the webhook in the RevenueCat dashboard (REVENUECAT_WEBHOOK_AUTH).
//
// Response codes matter: RevenueCat retries any non-2xx with backoff, so
// transient failures (DB down, product mapping not deployed yet) answer 500 to
// get the event redelivered, while conditions a retry can never fix (unknown
// user on this data plane, duplicate, ignored event type) answer 200.
func (s *Server) PostWebhooksRevenuecat(w http.ResponseWriter, r *http.Request) {
	cfg := config.AppConfig
	if cfg == nil || cfg.RevenueCatWebhookAuth == "" {
		s.logger.Error("revenuecat webhook: REVENUECAT_WEBHOOK_AUTH not configured")
		http.Error(w, "revenuecat webhook not configured", http.StatusServiceUnavailable)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(cfg.RevenueCatWebhookAuth)) != 1 {
		s.logger.Warn("revenuecat webhook: invalid authorization header")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var envelope revenueCatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		// Still 200: RevenueCat would otherwise retry a payload we can never parse.
		s.logger.Warn("revenuecat webhook: malformed payload", zap.Error(err))
		writeRevenueCatAck(w, "malformed")
		return
	}

	event := envelope.Event
	logger := s.logger.With(
		zap.String("event_id", event.ID),
		zap.String("event_type", event.Type),
		zap.String("product_id", event.ProductID),
		zap.String("store", event.Store),
	)

	txType, isCredit := revenueCatCreditTypes[event.Type]
	// Refunds have no event type of their own: they arrive as CANCELLATION with
	// cancel_reason CUSTOMER_SUPPORT ("customer received a refund"), for
	// subscriptions and one-time purchases alike. Every other cancel reason is an
	// auto-renew change — the user keeps what the purchase granted, and simply
	// stops being credited more.
	isRefund := strings.EqualFold(event.Type, "CANCELLATION") &&
		strings.EqualFold(event.CancelReason, "CUSTOMER_SUPPORT")
	if !isCredit && !isRefund {
		logger.Info("revenuecat webhook: event type carries no balance action; acked")
		writeRevenueCatAck(w, "ignored")
		return
	}
	// Store-sandbox purchases must never mint real minutes on the plane that takes
	// real money — and only there. The QA planes run as ENVIRONMENT=qa, where
	// sandbox purchases are the only purchases there are and must credit; for a
	// while QA ran as "production" (there was no third value) and this guard
	// dropped its test subscriptions — acked 200, never retried, silently gone.
	// See the ENVIRONMENT taxonomy in config.LoadConfig.
	if strings.EqualFold(event.Environment, "SANDBOX") && cfg.Environment == "production" {
		logger.Warn("revenuecat webhook: sandbox purchase dropped in production")
		writeRevenueCatAck(w, "sandbox")
		return
	}

	userID, err := resolveRevenueCatUser(event)
	if err != nil {
		logger.Error("revenuecat webhook: user lookup failed", zap.Error(err))
		http.Error(w, "user lookup failed", http.StatusInternalServerError)
		return
	}
	if userID == "" {
		// Not an error: with one webhook per regional data plane, the plane that
		// doesn't own the user sees the same event and must drop it silently.
		logger.Info("revenuecat webhook: no matching user on this data plane; acked",
			zap.String("app_user_id", event.AppUserID))
		writeRevenueCatAck(w, "unknown_user")
		return
	}

	amountSeconds, known := cfg.RevenueCatProductSeconds(event.ProductID)
	if !known {
		// 500 on purpose: this is a config gap (REVENUECAT_PRODUCT_MINUTES), and
		// RevenueCat's retries give the fixed mapping a chance to catch the event.
		logger.Error("revenuecat webhook: product missing from REVENUECAT_PRODUCT_MINUTES; answering 500 for retry")
		http.Error(w, "unmapped product", http.StatusInternalServerError)
		return
	}

	referenceID := "revenuecat:" + event.ID
	if event.ID == "" {
		if event.TransactionID == "" {
			logger.Warn("revenuecat webhook: event has neither id nor transaction_id; acked")
			writeRevenueCatAck(w, "unidentifiable")
			return
		}
		// The refund fallback is namespaced apart from the credit's: a refund shares
		// its transaction_id with the purchase it undoes, and on the shared key the
		// unique index would swallow the claw-back as a "duplicate" of the credit.
		if isRefund {
			referenceID = "revenuecat:refund:tx:" + event.TransactionID
		} else {
			referenceID = "revenuecat:tx:" + event.TransactionID
		}
	}

	product := event.ProductID
	description := revenueCatDescription(event)

	var entry *models.BalanceTransaction
	if isRefund {
		// The full grant comes back out, spent or not — the store returned the
		// money, and a balance pushed negative is the account's honest position.
		entry, err = balance.RefundPurchase(r.Context(), userID, amountSeconds, &product, &description, referenceID)
	} else {
		entry, err = balance.CreditPurchase(r.Context(), userID, amountSeconds, txType, &product, &description, referenceID)
	}
	switch {
	case errors.Is(err, balance.ErrDuplicateReference):
		logger.Info("revenuecat webhook: duplicate delivery dropped")
		writeRevenueCatAck(w, "duplicate")
		return
	case errors.Is(err, gorm.ErrRecordNotFound):
		// The user existed a moment ago and is gone now (account deletion racing
		// the webhook). A retry can never succeed.
		logger.Info("revenuecat webhook: user deleted before credit; acked")
		writeRevenueCatAck(w, "unknown_user")
		return
	case err != nil:
		logger.Error("revenuecat webhook: credit failed", zap.Error(err))
		http.Error(w, "credit failed", http.StatusInternalServerError)
		return
	}

	if isRefund {
		logger.Info("revenuecat webhook: purchase refunded, minutes clawed back",
			zap.String("user_id", userID),
			zap.Int64("amount_seconds", amountSeconds),
			zap.Int64("balance_after", entry.BalanceAfter))
		writeRevenueCatAck(w, "refunded")
		return
	}
	logger.Info("revenuecat webhook: balance credited",
		zap.String("user_id", userID),
		zap.Int64("amount_seconds", amountSeconds),
		zap.Int64("balance_after", entry.BalanceAfter))
	writeRevenueCatAck(w, "credited")
}

// resolveRevenueCatUser finds which local user the event belongs to, trying the
// current app_user_id first, then the original id, then every merged alias.
// RevenueCat's anonymous placeholder ids can never match and are skipped.
// Returns "" when no candidate exists in this plane's database.
func resolveRevenueCatUser(event revenueCatEvent) (string, error) {
	candidates := append([]string{event.AppUserID, event.OriginalAppUserID}, event.Aliases...)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] || strings.HasPrefix(candidate, "$RCAnonymousID:") {
			continue
		}
		seen[candidate] = true
		var user models.User
		err := database.DB.Select("id").Where("id = ?", candidate).First(&user).Error
		if err == nil {
			return user.ID, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}
	return "", nil
}

// revenueCatDescription condenses the event into a human-readable ledger note,
// e.g. "revenuecat renewal (app_store, normal) 9.99 USD".
func revenueCatDescription(event revenueCatEvent) string {
	desc := "revenuecat " + strings.ToLower(event.Type)
	var qualifiers []string
	if event.Store != "" {
		qualifiers = append(qualifiers, strings.ToLower(event.Store))
	}
	if event.PeriodType != "" {
		qualifiers = append(qualifiers, strings.ToLower(event.PeriodType))
	}
	if event.CancelReason != "" {
		qualifiers = append(qualifiers, strings.ToLower(event.CancelReason))
	}
	if len(qualifiers) > 0 {
		desc += " (" + strings.Join(qualifiers, ", ") + ")"
	}
	if event.Price != nil && event.Currency != "" {
		desc += fmt.Sprintf(" %.2f %s", *event.Price, event.Currency)
	}
	return desc
}

func writeRevenueCatAck(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}
