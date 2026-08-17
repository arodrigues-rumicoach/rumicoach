package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BalanceTransactionType identifies what a ledger row represents.
type BalanceTransactionType string

const (
	BalanceTxSubscription BalanceTransactionType = "subscription"
	BalanceTxTopUp        BalanceTransactionType = "top_up"
	BalanceTxSessionUsage BalanceTransactionType = "session_usage"
	// BalanceTxSessionFree marks a session the user was not charged for — one of the
	// introductory ones. AmountSeconds is 0, so it moves no balance and the
	// balance = SUM(amount_seconds) invariant is untouched.
	//
	// It exists because the ABSENCE of a debit says nothing: a session with no
	// session_usage row might have been free, or its debit might have failed (the
	// debit is log-and-continue), or it might have lasted under a second. Only an
	// explicit row can tell the usage history "this happened and it was free" — and
	// the unique session_id index still holds, so a session has either a debit or
	// this, never both.
	BalanceTxSessionFree BalanceTransactionType = "session_free"
	// BalanceTxMessageUsage is one companion reply charged to the user. Only replies
	// to something the user sent are charged: a proactive nudge, a re-engagement
	// template or a system notice is our idea, not theirs, and billing someone for
	// a message they did not ask for is indefensible.
	BalanceTxMessageUsage BalanceTransactionType = "message_usage"
	// BalanceTxRefund claws back the minutes a purchase granted after the store
	// refunds it. Negative amount, may push the balance below zero when the minutes
	// were already spent — that is the honest state of an account that consumed what
	// it was refunded for.
	BalanceTxRefund BalanceTransactionType = "refund"
)

// BalanceTransaction is one immutable ledger row of the minutes bank.
// AmountSeconds is signed (positive = credit, negative = debit); BalanceAfter
// is users.balance_seconds after this row was applied, bank-statement style.
// Rows are only ever inserted, inside the same DB transaction that updates
// users.balance_seconds.
type BalanceTransaction struct {
	ID            string                 `gorm:"primaryKey;type:text"`
	UserID        string                 `gorm:"not null;type:text;index:idx_balance_tx_user_created,priority:1"`
	Type          BalanceTransactionType `gorm:"not null;type:text"`
	AmountSeconds int64                  `gorm:"not null"`
	BalanceAfter  int64                  `gorm:"not null"`
	// SessionID links session_usage debits to communication_sessions.id.
	// The unique index guarantees at most one debit per session (NULLs don't
	// collide in Postgres).
	SessionID   *string `gorm:"type:text;uniqueIndex"`
	SessionType *string `gorm:"type:text"`
	Product     *string `gorm:"type:text"` // plan/package label for credits
	// ReferenceID is the payment provider's idempotency key for credits (e.g.
	// "revenuecat:<event id>"). The unique index guarantees a redelivered
	// webhook can never credit the same purchase twice (NULLs don't collide
	// in Postgres, so manual/admin credits are unaffected).
	ReferenceID *string   `gorm:"type:text;uniqueIndex"`
	Description *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime;type:timestamp with time zone;index:idx_balance_tx_user_created,priority:2,sort:desc"`
}

func (t *BalanceTransaction) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return
}
