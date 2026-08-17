// Package balance is the minutes bank: it owns users.balance_seconds and the
// immutable balance_transactions ledger. Every mutation goes through one DB
// transaction that locks the user row, updates the balance, and inserts the
// ledger entry, so the balance always equals the sum of the ledger.
package balance

import (
	"context"
	"errors"
	"fmt"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/journey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrDuplicateReference reports that a credit carrying the same referenceID was
// already applied — a redelivered provider webhook, not a failure.
var ErrDuplicateReference = errors.New("balance: reference already credited")

// MinimumStartSeconds is the balance a user must still hold to be allowed to open
// a billable session. It is a full minute rather than a single second because a
// session that dies seconds after the greeting is worse than a paywall: the user
// pays attention, gets nothing, and the balance goes negative anyway (debits are
// recorded at session end and may overdraw).
const MinimumStartSeconds int64 = 60

// FreeSessionCap bounds the free introductory sessions in the one direction the
// rule below cannot bound itself. That rule asks whether the opening pair has
// produced its two artifacts, and a user who never lets it finish never stops
// qualifying — the coaching model can be talked to indefinitely for nothing. Ten
// substantive sessions is far past any honest onboarding (it takes two) and far
// short of a bill worth worrying about, so a real user never meets this and a
// scripted one stops here.
//
// Counted from sessions that pass journey.SessionCountsAsDone and NOT scoped with
// models.JourneySessions, both on purpose: an abandoned start costs nothing, and
// erasing journey progress does not refill the allowance. This is the backstop, so
// nothing the client can do may reset it.
const FreeSessionCap int64 = 10

// MessageCostSeconds is the flat fee for a TEXT reply, in the same currency as
// everything else in this ledger: seconds of coaching time. Typed messages have no
// natural length, so they are priced at a fraction of a minute's attention.
//
// Spoken messages are NOT priced from this — a voice note is charged by how long it
// runs, on both sides, which is the honest measure of what was said and heard. See
// companion.Spend.ChargeSeconds, which composes the two rules.
//
// It lives here, next to the debit that applies it, rather than in the usage feed
// that displays it — the feed used to own it and multiply it out at read time, which
// meant the number on screen was never anything the balance had actually lost.
const MessageCostSeconds int64 = 5

// IsIntroductorySession reports whether a session type is one of the two the free
// allowance covers: the onboarding intro and the Vision session it hands over to.
// Everything else — check-ins and the later deep sessions — is paid for out of the
// balance, always.
//
// The argument must be the type the SERVER resolved (chat.resolveSessionType), never
// the one the client asked for. A client that could name its own session type could
// name a free one.
func IsIntroductorySession(sessionType string) bool {
	return sessionType == string(api.SessionTypeOnboarding) ||
		sessionType == string(api.SessionTypeSessionVision)
}

// FreeSessionAvailable reports whether a session is one of the free introductory ones:
// the right kind of session, and the opening pair still unfinished.
//
// Two conditions, deliberately separate. The kind matters because a check-in is never
// free whatever state the account is in; the state matters because the intro and Vision
// stop being free once they have done their job. In practice the second implies the
// first — a user with an unfinished opening pair is always routed to onboarding/Vision,
// see resolveSessionType — but the rule does not lean on that: this is where money is
// decided, and it should be readable without tracing the routing.
//
// On a query error it returns false, err. Callers decide — the WebSocket pre-flight
// fails open on a database hiccup rather than locking people out, and the debit path
// fails closed rather than giving sessions away; both say so at their call sites.
func FreeSessionAvailable(ctx context.Context, userID, sessionType string) (bool, error) {
	if !IsIntroductorySession(sessionType) {
		return false, nil
	}
	return OpeningPairUnfinished(ctx, userID)
}

// OpeningPairUnfinished reports whether the introductory sessions still have work to
// do, asking that of the work itself rather than of a counter.
//
// The opening pair exists to produce exactly two things: the profile details the
// intro collects (country, date of birth, gender) and the ideal-life vision the
// Vision session writes. While either is missing the pair is unfinished; once both
// exist the introduction is over.
//
// Two earlier rules are worth not repeating. The first asked users.state, which
// defaults to VISION_IDEAL_LIFE on every new account and could only be left by
// finishing Vision cleanly, so anyone who hung up part-way stayed exempt forever.
// The second counted rows in communication_sessions — but the row is inserted at
// connect time, so a five-second connection that produced nothing burned a free
// session and dropped the user onto a paywall mid-onboarding. Keying on the
// artifacts fixes both: it cannot stall the way a state transition can, and it
// cannot be consumed by a session that did not happen.
//
// Separate from FreeSessionAvailable because two callers need this question without a
// session type in hand: the WebSocket pre-flight, which runs before the type is
// resolved, and /me's inFirstJourney, which is about the account rather than any one
// session.
func OpeningPairUnfinished(ctx context.Context, userID string) (bool, error) {
	var user models.User
	if err := database.DB.WithContext(ctx).
		Select("id", "date_of_birth", "gender", "country", "ideal_life_vision_set_at").
		Where("id = ?", userID).First(&user).Error; err != nil {
		return false, err
	}

	// Both artifacts on record: the introduction is done, whatever the history says.
	if !user.NeedsProfileDetails() && user.IdealLifeVisionSetAt != nil {
		return false, nil
	}

	type sessionRecord struct {
		SessionType string
		Duration    int
	}
	var records []sessionRecord
	if err := database.DB.WithContext(ctx).Model(&models.CommunicationSession{}).
		Select("session_type, duration").
		Where("user_id = ? AND session_type IS NOT NULL", userID).
		Scan(&records).Error; err != nil {
		return false, err
	}

	var done int64
	for _, r := range records {
		if journey.SessionCountsAsDone(r.SessionType, r.Duration) {
			done++
		}
	}
	return done < FreeSessionCap, nil
}

// Credit adds purchased seconds to a user's balance. amountSeconds must be
// positive and txType must be a purchase type (subscription or top_up). Used
// for manual/admin credits; provider webhooks use CreditPurchase instead so
// redeliveries dedupe on the provider's event id.
func Credit(ctx context.Context, userID string, amountSeconds int64, txType models.BalanceTransactionType, product, description *string) (*models.BalanceTransaction, error) {
	if amountSeconds <= 0 {
		return nil, fmt.Errorf("credit amount must be positive, got %d", amountSeconds)
	}
	if txType != models.BalanceTxSubscription && txType != models.BalanceTxTopUp {
		return nil, fmt.Errorf("invalid credit type %q", txType)
	}
	return apply(ctx, userID, amountSeconds, txType, nil, nil, product, description, nil)
}

// CreditPurchase is Credit keyed by a payment provider's event id: at most one
// ledger row per referenceID ever exists (checked under the user-row lock and
// backstopped by the unique reference_id index), so webhook redeliveries return
// ErrDuplicateReference instead of crediting twice.
func CreditPurchase(ctx context.Context, userID string, amountSeconds int64, txType models.BalanceTransactionType, product, description *string, referenceID string) (*models.BalanceTransaction, error) {
	if amountSeconds <= 0 {
		return nil, fmt.Errorf("credit amount must be positive, got %d", amountSeconds)
	}
	if txType != models.BalanceTxSubscription && txType != models.BalanceTxTopUp {
		return nil, fmt.Errorf("invalid credit type %q", txType)
	}
	if referenceID == "" {
		return nil, fmt.Errorf("referenceID must not be empty")
	}
	return apply(ctx, userID, amountSeconds, txType, nil, nil, product, description, &referenceID)
}

// DebitSession records a session's usage against the balance. elapsedSeconds
// of zero or less is a no-op (returns nil, nil). A second debit for the same
// sessionID fails on the ledger's unique session_id index, so a session can
// never be charged twice. The balance may go negative.
func DebitSession(ctx context.Context, userID, sessionID string, sessionType *string, elapsedSeconds int64) (*models.BalanceTransaction, error) {
	if elapsedSeconds <= 0 {
		return nil, nil
	}
	return apply(ctx, userID, -elapsedSeconds, models.BalanceTxSessionUsage, &sessionID, sessionType, nil, nil, nil)
}

// ListTransactions returns one page of the user's ledger, newest first, plus
// the total row count for pagination.
func ListTransactions(ctx context.Context, userID string, page, limit int) ([]models.BalanceTransaction, int64, error) {
	var total int64
	if err := database.DB.WithContext(ctx).Model(&models.BalanceTransaction{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []models.BalanceTransaction
	if err := database.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// apply atomically shifts the user's balance by delta and inserts the matching
// ledger row. The SELECT ... FOR UPDATE on the users row serializes concurrent
// mutations (e.g. an HTTP credit racing a WS session debit) for the same user.
func apply(ctx context.Context, userID string, delta int64, txType models.BalanceTransactionType, sessionID, sessionType, product, description, referenceID *string) (*models.BalanceTransaction, error) {
	var entry models.BalanceTransaction
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("id = ?", userID)
		// SQLite (in-memory test DB) has no FOR UPDATE syntax; its writers are
		// serialized anyway, so the lock is only needed on Postgres.
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var user models.User
		if err := q.First(&user).Error; err != nil {
			return err
		}

		// Provider idempotency: a reference always belongs to one user, and that
		// user's row is locked above, so check-then-insert cannot race. The
		// unique index on reference_id is the cross-user/crash backstop.
		if referenceID != nil {
			var existing int64
			if err := tx.Model(&models.BalanceTransaction{}).
				Where("reference_id = ?", *referenceID).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return ErrDuplicateReference
			}
		}

		newBalance := user.BalanceSeconds + delta // may go negative on debit — intentional
		// Raw SQL because the model declares balance_seconds <-:false, which is what
		// stops every OTHER write path from touching the column. This statement and
		// the QA account reset are deliberately the only two ways through — a
		// single-column update either way, so concurrent writes to other user fields
		// (state, theme, session handles) can't be clobbered.
		if err := tx.Exec(`UPDATE users SET balance_seconds = ? WHERE id = ?`,
			newBalance, userID).Error; err != nil {
			return err
		}

		entry = models.BalanceTransaction{
			UserID:        userID,
			Type:          txType,
			AmountSeconds: delta,
			BalanceAfter:  newBalance,
			SessionID:     sessionID,
			SessionType:   sessionType,
			Product:       product,
			ReferenceID:   referenceID,
			Description:   description,
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// RecordFreeSession notes that a session ran and was not charged for, as a
// zero-amount ledger row.
//
// The row carries no money — it is there so the usage history can show the session
// at all. Nothing else can: a session with no debit is indistinguishable from one
// whose debit failed, since DebitSession is log-and-continue at the call site, and
// from one that lasted under a second, which DebitSession drops. The user's own
// history should account for every session they had, including the ones we gave them.
//
// The unique session_id index means this and a debit are mutually exclusive, which is
// exactly the intent: a session is charged or it is free, never recorded as both.
// A duplicate call therefore fails on the index, and callers log and continue.
func RecordFreeSession(ctx context.Context, userID, sessionID string, sessionType *string) (*models.BalanceTransaction, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID must not be empty")
	}
	return apply(ctx, userID, 0, models.BalanceTxSessionFree, &sessionID, sessionType, nil, nil, nil)
}

// DebitMessage charges a user for one reply, keyed to the inbound message it answers.
//
// Only replies reach here: a proactive message, a re-engagement template and a system
// notice are all things we decided to send, and charging for those would bill the user
// for our own initiative. See deliverReply, which is where the distinction is visible.
//
// seconds is what the exchange came to — a flat fee for text, the audio's real length
// when either side was spoken (companion.Spend.ChargeSeconds decides). A non-positive
// amount is a no-op rather than an error: nothing measurable happened, so there is
// nothing to charge and no row worth writing.
//
// inboundMessageID is the message being answered, not the reply, and it becomes the
// reference_id. That makes the charge idempotent on the thing that can actually repeat:
// providers redeliver inbound events, and a redelivery that slipped past the webhook
// dedupe would otherwise be answered and charged twice. A second attempt returns
// ErrDuplicateReference, which callers treat as "already charged", not as a failure.
func DebitMessage(ctx context.Context, userID, inboundMessageID string, seconds int64) (*models.BalanceTransaction, error) {
	if inboundMessageID == "" {
		return nil, fmt.Errorf("inboundMessageID must not be empty")
	}
	if seconds <= 0 {
		return nil, nil
	}
	ref := "message:" + inboundMessageID
	return apply(ctx, userID, -seconds, models.BalanceTxMessageUsage, nil, nil, nil, nil, &ref)
}

// RefundPurchase claws back the minutes a refunded purchase granted.
//
// The full product amount, not "what is left": the store gave the money back, so the
// minutes it bought are withdrawn, and if they were already spent the balance goes
// negative — which is exactly the account's honest position. amountSeconds is the
// product's mapped grant, positive, applied here as a debit.
//
// referenceID is the refund event's own id (webhook redeliveries return
// ErrDuplicateReference instead of clawing back twice). It must differ from the
// original credit's reference — the unique index would otherwise swallow the refund
// as a "duplicate" of the purchase it undoes.
func RefundPurchase(ctx context.Context, userID string, amountSeconds int64, product, description *string, referenceID string) (*models.BalanceTransaction, error) {
	if amountSeconds <= 0 {
		return nil, fmt.Errorf("refund amount must be positive, got %d", amountSeconds)
	}
	if referenceID == "" {
		return nil, fmt.Errorf("referenceID must not be empty")
	}
	return apply(ctx, userID, -amountSeconds, models.BalanceTxRefund, nil, nil, product, description, &referenceID)
}
