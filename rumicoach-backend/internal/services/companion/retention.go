package companion

import (
	"fmt"

	"github.com/rumi/rumi-be/internal/models"
	"gorm.io/gorm"
)

// followUpRetentionDays bounds how long a drained channel_follow_ups row is kept. Drained
// rows now carry sent_text — the delivered copy of each proactive message, kept as the
// anti-repetition memory for later generations — so this sweep is a privacy bound, not
// just table hygiene. It stays a constant rather than a user setting because the window
// only needs to cover what recentProactiveBlock reads (14 days), with margin.
const followUpRetentionDays = 30

// SetChatHistoryRetention changes how long a user's chat history is kept AND brings their
// stored messages in line with the new setting, in one transaction.
//
// The rewrite is the whole point and must never be skipped. Every message carries its own
// ExpiresAt, materialized when it was stored, so changing only users.chat_history_retention_days
// would leave the entire existing conversation living by the old rule — and lowering the
// retention is exactly when someone expects the change to bite immediately and
// retroactively. This function is the only sanctioned way to change the value.
func SetChatHistoryRetention(db *gorm.DB, userID string, days int) error {
	if !models.ValidChatRetention(days) {
		return fmt.Errorf("invalid chat history retention: %d days", days)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).
			Update("chat_history_retention_days", days).Error; err != nil {
			return err
		}

		if days <= 0 {
			// Kept indefinitely: clear the expiries rather than pushing them far out, so
			// "never" is represented as absence and cannot quietly come due later.
			return tx.Model(&models.ChannelMessage{}).Where("user_id = ?", userID).
				Update("expires_at", nil).Error
		}

		// Recomputed from each message's own created_at, so the new rule applies to the
		// history as if it had always been in force. Messages already past the new horizon
		// land in the past and go on the next sweep.
		return tx.Model(&models.ChannelMessage{}).Where("user_id = ?", userID).
			Update("expires_at", gorm.Expr(expiryExpr(tx), days)).Error
	})
}

// expiryExpr is created_at plus n days, in the dialect at hand. Postgres runs the product;
// SQLite backs the tests, and its date arithmetic is spelled differently enough that a
// single expression cannot serve both.
func expiryExpr(tx *gorm.DB) string {
	if tx.Dialector.Name() == "postgres" {
		return "created_at + make_interval(days => ?)"
	}
	return "datetime(created_at, '+' || ? || ' days')"
}
