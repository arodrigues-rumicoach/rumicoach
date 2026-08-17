// Package usage builds the usage-history feed for the Subscription & usage screen:
// what the user has spent, not what was said and not everything that happened.
//
// It reads balance_transactions, which is the only table that knows what was actually
// charged, and reaches into communication_sessions for one thing only — how long a
// free session ran, so it can be shown at its real duration. It never reads
// channel_messages: that log is erasable three ways over and could not be a billing
// source even if it survived.
package usage

import (
	"context"
	"sort"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
)

// Entry is one row of the usage feed: either a billed session or one calendar
// day's worth of companion messages.
type Entry struct {
	Kind       string // "session" | "messages"
	OccurredAt time.Time
	// Seconds is how long the entry ran. For a free session that is its real
	// duration, NOT what came off the balance — see Free.
	Seconds int64
	// Free marks a session the user was not charged for. Session entries only.
	Free         bool
	SessionID    *string // session entries only
	SessionType  *string // session entries only
	Day          string  // messages entries only, YYYY-MM-DD in the requested location
	MessageCount int     // messages entries only
}

// Totals aggregates the whole history, independent of pagination.
type Totals struct {
	TotalSeconds int64
	// SessionSeconds is what was actually charged; free sessions contribute nothing
	// to it, and their duration lands in FreeSessionSeconds instead.
	SessionSeconds     int64
	FreeSessionSeconds int64
	// SessionCount is every session in the feed, free ones included.
	SessionCount   int
	MessageSeconds int64
	// MessageCount is charged replies, not every message the user received.
	MessageCount int
}

// History returns the full usage feed newest-first plus its totals.
//
// Sessions come from the ledger's session rows: session_usage debits, plus the
// zero-amount session_free rows that mark the introductory sessions. The free ones
// are shown rather than hidden — a history that silently omitted the user's first two
// sessions would be describing someone else's account — but they carry Free and add
// nothing to SessionSeconds, so "what you spent" stays honest.
//
// A free row has no amount to read a duration from, so those come from
// communication_sessions. That table is the one thing this package otherwise refuses
// to read, and the reason does not apply here: the ban exists because it is erasable
// and so cannot be a BILLING source, whereas this is a duration for display, and the
// erasure redacts content while leaving the timings (see redactSessionContent). A
// session row that is somehow gone leaves the entry at zero seconds rather than
// dropping it.
//
// Messages are the message_usage debits, grouped per calendar day in loc: the replies
// the user was charged for, and nothing else. What we send on our own initiative —
// proactive nudges, re-engagement templates, system notices — costs them nothing and
// so has no place on a bill.
//
// The cost used to be a flat rate multiplied out here at read time over every outbound
// message, which meant the figure on screen was never something the balance had
// actually lost, and counted messages the user was never charged for. Both halves now
// come from the ledger, so the screen and the balance cannot disagree.
//
// The merge happens in Go rather than a SQL UNION: both sources are small
// (a few rows per session / per active day), and the Go path keeps the query
// portable across Postgres and the SQLite test DB. Callers paginate the slice.
func History(ctx context.Context, userID string, loc *time.Location) ([]Entry, Totals, error) {
	if loc == nil {
		loc = time.UTC
	}

	var sessionRows []models.BalanceTransaction
	if err := database.DB.WithContext(ctx).
		Where("user_id = ? AND type IN ?", userID,
			[]models.BalanceTransactionType{models.BalanceTxSessionUsage, models.BalanceTxSessionFree}).
		Order("created_at desc").
		Find(&sessionRows).Error; err != nil {
		return nil, Totals{}, err
	}

	freeDurations, err := freeSessionDurations(ctx, sessionRows)
	if err != nil {
		return nil, Totals{}, err
	}

	var messageDebits []models.BalanceTransaction
	if err := database.DB.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, models.BalanceTxMessageUsage).
		Find(&messageDebits).Error; err != nil {
		return nil, Totals{}, err
	}

	var totals Totals
	entries := make([]Entry, 0, len(sessionRows))

	for _, tx := range sessionRows {
		free := tx.Type == models.BalanceTxSessionFree
		seconds := -tx.AmountSeconds // debits are negative in the ledger
		if free {
			// A free row carries no amount; its duration comes from the session.
			seconds = 0
			if tx.SessionID != nil {
				seconds = freeDurations[*tx.SessionID]
			}
		}
		entries = append(entries, Entry{
			Kind:        "session",
			OccurredAt:  tx.CreatedAt,
			Seconds:     seconds,
			Free:        free,
			SessionID:   tx.SessionID,
			SessionType: tx.SessionType,
		})
		if free {
			totals.FreeSessionSeconds += seconds
		} else {
			totals.SessionSeconds += seconds
		}
		totals.SessionCount++
	}

	byDay := make(map[string]*Entry)
	for _, tx := range messageDebits {
		day := tx.CreatedAt.In(loc).Format("2006-01-02")
		e, ok := byDay[day]
		if !ok {
			e = &Entry{Kind: "messages", Day: day}
			byDay[day] = e
		}
		seconds := -tx.AmountSeconds
		e.MessageCount++
		e.Seconds += seconds
		if tx.CreatedAt.After(e.OccurredAt) {
			e.OccurredAt = tx.CreatedAt
		}
		totals.MessageSeconds += seconds
		totals.MessageCount++
	}
	for _, e := range byDay {
		entries = append(entries, *e)
	}

	// Deliberately excludes FreeSessionSeconds: this is what the user has spent, and
	// the free sessions are precisely the ones they did not.
	totals.TotalSeconds = totals.SessionSeconds + totals.MessageSeconds

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].OccurredAt.After(entries[j].OccurredAt)
	})
	return entries, totals, nil
}

// freeSessionDurations looks up how long each free session ran, keyed by session id.
// One query for the whole page rather than one per row.
func freeSessionDurations(ctx context.Context, rows []models.BalanceTransaction) (map[string]int64, error) {
	var ids []string
	for _, tx := range rows {
		if tx.Type == models.BalanceTxSessionFree && tx.SessionID != nil {
			ids = append(ids, *tx.SessionID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var found []struct {
		ID       string
		Duration int
	}
	if err := database.DB.WithContext(ctx).Model(&models.CommunicationSession{}).
		Select("id, duration").Where("id IN ?", ids).Scan(&found).Error; err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(found))
	for _, f := range found {
		out[f.ID] = int64(f.Duration)
	}
	return out, nil
}
