package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"go.uber.org/zap"
)

const rcTestAuth = "Bearer rc-webhook-secret"

func setupRevenueCatTest(t *testing.T) *Server {
	t.Helper()
	setupBalanceTestDB(t)

	previous := config.AppConfig
	config.AppConfig = &config.Config{
		Environment:              "development",
		RevenueCatWebhookAuth:    rcTestAuth,
		RevenueCatProductMinutes: "rumi_monthly_120:120,rumi_topup_60:60",
	}
	t.Cleanup(func() { config.AppConfig = previous })

	if err := database.DB.Exec(`INSERT INTO users (id, balance_seconds) VALUES ('user-123', 0)`).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return NewServer(zap.NewNop(), nil, nil, nil)
}

func postRevenueCat(t *testing.T, server *Server, auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/webhooks/revenuecat", bytes.NewBufferString(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	server.PostWebhooksRevenuecat(w, req)
	return w
}

func rcEventBody(eventID, eventType, appUserID, productID string) string {
	return fmt.Sprintf(`{
		"api_version": "1.0",
		"event": {
			"id": %q,
			"type": %q,
			"app_user_id": %q,
			"product_id": %q,
			"period_type": "NORMAL",
			"store": "APP_STORE",
			"environment": "SANDBOX",
			"transaction_id": "tx-1",
			"price": 9.99,
			"currency": "USD"
		}
	}`, eventID, eventType, appUserID, productID)
}

func rcUserBalance(t *testing.T, userID string) int64 {
	t.Helper()
	var balanceSeconds int64
	if err := database.DB.Raw(`SELECT balance_seconds FROM users WHERE id = ?`, userID).Scan(&balanceSeconds).Error; err != nil {
		t.Fatalf("failed to read balance: %v", err)
	}
	return balanceSeconds
}

func TestPostWebhooksRevenuecat(t *testing.T) {
	server := setupRevenueCatTest(t)

	t.Run("Missing auth rejected", func(t *testing.T) {
		w := postRevenueCat(t, server, "", rcEventBody("evt-1", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", w.Code)
		}
	})

	t.Run("Wrong auth rejected", func(t *testing.T) {
		w := postRevenueCat(t, server, "Bearer wrong", rcEventBody("evt-1", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", w.Code)
		}
		if rcUserBalance(t, "user-123") != 0 {
			t.Error("Balance must not change on rejected request")
		}
	})

	t.Run("Initial purchase credits subscription minutes", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-1", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 7200 {
			t.Errorf("Expected balance 7200, got %d", got)
		}
		var txType, product, reference string
		row := database.DB.Raw(`SELECT type, product, reference_id FROM balance_transactions WHERE reference_id = 'revenuecat:evt-1'`).Row()
		if err := row.Scan(&txType, &product, &reference); err != nil {
			t.Fatalf("ledger row missing: %v", err)
		}
		if txType != "subscription" || product != "rumi_monthly_120" {
			t.Errorf("Unexpected ledger row: type=%s product=%s", txType, product)
		}
	})

	t.Run("Redelivered event does not double credit", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-1", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 on duplicate, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 7200 {
			t.Errorf("Expected balance still 7200 after redelivery, got %d", got)
		}
		var rows int64
		database.DB.Raw(`SELECT COUNT(*) FROM balance_transactions`).Scan(&rows)
		if rows != 1 {
			t.Errorf("Expected 1 ledger row, got %d", rows)
		}
	})

	t.Run("Non-renewing purchase credits top-up minutes", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-2", "NON_RENEWING_PURCHASE", "user-123", "rumi_topup_60"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 7200+3600 {
			t.Errorf("Expected balance 10800, got %d", got)
		}
		var txType string
		database.DB.Raw(`SELECT type FROM balance_transactions WHERE reference_id = 'revenuecat:evt-2'`).Scan(&txType)
		if txType != "top_up" {
			t.Errorf("Expected top_up ledger type, got %s", txType)
		}
	})

	t.Run("Renewal credits again under a new event id", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-3", "RENEWAL", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 10800+7200 {
			t.Errorf("Expected balance 18000, got %d", got)
		}
	})

	t.Run("Non-purchase event acked without credit", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-4", "CANCELLATION", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		if got := rcUserBalance(t, "user-123"); got != 18000 {
			t.Errorf("Expected balance unchanged at 18000, got %d", got)
		}
	})

	t.Run("Unknown product answers 500 for retry", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-5", "INITIAL_PURCHASE", "user-123", "unmapped_product"))
		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected 500, got %d", w.Code)
		}
		if got := rcUserBalance(t, "user-123"); got != 18000 {
			t.Errorf("Expected balance unchanged at 18000, got %d", got)
		}
	})

	t.Run("Unknown user acked without credit", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-6", "INITIAL_PURCHASE", "someone-else", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 (other data plane may own the user), got %d", w.Code)
		}
		var rows int64
		database.DB.Raw(`SELECT COUNT(*) FROM balance_transactions WHERE reference_id = 'revenuecat:evt-6'`).Scan(&rows)
		if rows != 0 {
			t.Error("No ledger row may exist for an unknown user")
		}
	})

	t.Run("User resolved through aliases", func(t *testing.T) {
		body := `{
			"api_version": "1.0",
			"event": {
				"id": "evt-7",
				"type": "INITIAL_PURCHASE",
				"app_user_id": "$RCAnonymousID:abc",
				"original_app_user_id": "$RCAnonymousID:abc",
				"aliases": ["$RCAnonymousID:abc", "user-123"],
				"product_id": "rumi_topup_60",
				"store": "PLAY_STORE",
				"environment": "SANDBOX"
			}
		}`
		w := postRevenueCat(t, server, rcTestAuth, body)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 18000+3600 {
			t.Errorf("Expected balance 21600, got %d", got)
		}
	})

	t.Run("Sandbox purchase dropped in production", func(t *testing.T) {
		config.AppConfig.Environment = "production"
		t.Cleanup(func() { config.AppConfig.Environment = "development" })
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-8", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d", w.Code)
		}
		if got := rcUserBalance(t, "user-123"); got != 21600 {
			t.Errorf("Expected balance unchanged at 21600, got %d", got)
		}
	})

	// The QA planes run as ENVIRONMENT=qa: hardened like production everywhere else,
	// but sandbox purchases credit — they are the only purchases QA ever sees. When
	// QA ran as "production" its test subscriptions were acked and silently never
	// credited, which is the regression this pins.
	t.Run("Sandbox purchase credited on a qa plane", func(t *testing.T) {
		config.AppConfig.Environment = "qa"
		t.Cleanup(func() { config.AppConfig.Environment = "development" })
		w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-9", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 21600+7200 {
			t.Errorf("Expected balance 28800, got %d", got)
		}
	})

	t.Run("Malformed payload acked", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, `{"event": not-json`)
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}
	})
}

func rcCancellationBody(eventID, appUserID, productID, cancelReason string) string {
	return fmt.Sprintf(`{
		"api_version": "1.0",
		"event": {
			"id": %q,
			"type": "CANCELLATION",
			"app_user_id": %q,
			"product_id": %q,
			"cancel_reason": %q,
			"period_type": "NORMAL",
			"store": "APP_STORE",
			"environment": "SANDBOX",
			"transaction_id": "tx-1",
			"price": 9.99,
			"currency": "USD"
		}
	}`, eventID, appUserID, productID, cancelReason)
}

// Refunds arrive as CANCELLATION + cancel_reason CUSTOMER_SUPPORT (there is no REFUND
// event type), and they claw the purchase's full grant back out of the balance.
func TestPostWebhooksRevenuecatRefund(t *testing.T) {
	server := setupRevenueCatTest(t)

	// The purchase being refunded: 120 minutes.
	w := postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-buy", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))
	if w.Code != http.StatusOK {
		t.Fatalf("credit failed: %d %s", w.Code, w.Body.String())
	}
	if got := rcUserBalance(t, "user-123"); got != 7200 {
		t.Fatalf("balance after purchase = %d, want 7200", got)
	}

	t.Run("Customer-support cancellation claws the grant back", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcCancellationBody("evt-refund", "user-123", "rumi_monthly_120", "CUSTOMER_SUPPORT"))
		if w.Code != http.StatusOK {
			t.Fatalf("refund failed: %d %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != 0 {
			t.Errorf("balance after refund = %d, want 0", got)
		}
	})

	t.Run("Redelivered refund cannot claw back twice", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcCancellationBody("evt-refund", "user-123", "rumi_monthly_120", "CUSTOMER_SUPPORT"))
		if w.Code != http.StatusOK {
			t.Fatalf("redelivery answered %d, want 200", w.Code)
		}
		if got := rcUserBalance(t, "user-123"); got != 0 {
			t.Errorf("balance after redelivery = %d, want 0 (debited once)", got)
		}
	})

	// A refund of minutes already spent leaves the account negative — the honest
	// position of someone who consumed what the store gave back.
	t.Run("Refund may push the balance negative", func(t *testing.T) {
		w := postRevenueCat(t, server, rcTestAuth, rcCancellationBody("evt-refund-2", "user-123", "rumi_topup_60", "CUSTOMER_SUPPORT"))
		if w.Code != http.StatusOK {
			t.Fatalf("refund failed: %d %s", w.Code, w.Body.String())
		}
		if got := rcUserBalance(t, "user-123"); got != -3600 {
			t.Errorf("balance = %d, want -3600", got)
		}
	})
}

// Turning off auto-renew is not a refund: the user keeps every minute already
// granted and simply stops being credited more.
func TestPostWebhooksRevenuecatUnsubscribeKeepsMinutes(t *testing.T) {
	server := setupRevenueCatTest(t)
	postRevenueCat(t, server, rcTestAuth, rcEventBody("evt-buy", "INITIAL_PURCHASE", "user-123", "rumi_monthly_120"))

	for _, reason := range []string{"UNSUBSCRIBE", "BILLING_ERROR", "PRICE_INCREASE", "UNKNOWN", ""} {
		w := postRevenueCat(t, server, rcTestAuth, rcCancellationBody("evt-c-"+reason, "user-123", "rumi_monthly_120", reason))
		if w.Code != http.StatusOK {
			t.Errorf("%q: answered %d, want 200 ack", reason, w.Code)
		}
	}
	if got := rcUserBalance(t, "user-123"); got != 7200 {
		t.Errorf("balance = %d, want 7200 untouched", got)
	}
}

// A refund for a product missing from REVENUECAT_PRODUCT_MINUTES is a config gap,
// answered 500 so RevenueCat's retries give the fixed mapping a chance — the same
// contract as the credit side.
func TestPostWebhooksRevenuecatRefundUnmappedProduct(t *testing.T) {
	server := setupRevenueCatTest(t)
	w := postRevenueCat(t, server, rcTestAuth, rcCancellationBody("evt-r", "user-123", "unknown_product", "CUSTOMER_SUPPORT"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("answered %d, want 500 for retry", w.Code)
	}
	if got := rcUserBalance(t, "user-123"); got != 0 {
		t.Errorf("balance = %d, want 0", got)
	}
}
