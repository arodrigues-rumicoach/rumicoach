package balance

import (
	"context"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
)

// StatementEntry is one row of the grouped statement view: a ledger row as-is,
// except that a run of message debits is folded into a single row. For a folded
// row the embedded transaction is the run's NEWEST debit — its id, balanceAfter
// and createdAt stand for the whole run — with AmountSeconds re-summed across
// the run, and Day/MessageCount say what was folded.
type StatementEntry struct {
	models.BalanceTransaction
	// Day is the run's calendar day (YYYY-MM-DD in the caller's location).
	// Message groups only; empty otherwise.
	Day string
	// MessageCount is how many charged replies the run collapses. Message
	// groups only; zero otherwise.
	MessageCount int
}

// Statement returns the user's full ledger newest-first, with consecutive
// message_usage debits on the same calendar day collapsed into one row each.
//
// Only CONSECUTIVE runs fold — any interleaved row (a session debit, a top-up)
// breaks the run. That is what keeps the statement arithmetically honest: each
// row's balanceAfter still equals the previous row's balanceAfter plus this
// row's amount, which grouping across an interleaved row would silently break.
// The cost is that a heavily interleaved day shows several message rows, which
// is rare and truthful.
//
// The whole ledger is fetched and folded in Go, like usage.History: page
// boundaries depend on how rows fold, so SQL-side pagination cannot know them,
// the per-user ledger is small, and the Go path stays portable across Postgres
// and the SQLite test DB. Callers paginate the slice.
func Statement(ctx context.Context, userID string, loc *time.Location) ([]StatementEntry, error) {
	if loc == nil {
		loc = time.UTC
	}

	var rows []models.BalanceTransaction
	if err := database.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	entries := make([]StatementEntry, 0, len(rows))
	for _, tx := range rows {
		if tx.Type == models.BalanceTxMessageUsage {
			day := tx.CreatedAt.In(loc).Format("2006-01-02")
			if n := len(entries); n > 0 {
				prev := &entries[n-1]
				// Rows arrive newest-first, so folding into the last emitted
				// entry is exactly "consecutive in the ledger". The group was
				// started by its newest row, whose id/balanceAfter/createdAt
				// already stand; older rows only add amount and count.
				if prev.Type == models.BalanceTxMessageUsage && prev.Day == day {
					prev.AmountSeconds += tx.AmountSeconds
					prev.MessageCount++
					continue
				}
			}
			entries = append(entries, StatementEntry{BalanceTransaction: tx, Day: day, MessageCount: 1})
			continue
		}
		entries = append(entries, StatementEntry{BalanceTransaction: tx})
	}
	return entries, nil
}
