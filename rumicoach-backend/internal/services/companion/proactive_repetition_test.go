package companion

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
)

// Proactive generation must see what it already sent, and the conversation log cannot
// provide that: the ephemeral purge erases channel_messages after a few quiet hours, and
// quiet users are exactly who proactive messages go to. The durable memory is
// channel_follow_ups.sent_text, and this pins the block that replays it into the directive.
func TestRecentProactiveBlock(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	long := time.Now().Add(-48 * time.Hour)
	addIntegration(t, "i1", &long)
	addIntegration(t, "i2", &long)

	// Nothing sent yet: no block, so the directive carries no empty scaffolding.
	if got := s.recentProactiveBlock("i1"); got != "" {
		t.Errorf("block with no history = %q, want empty", got)
	}

	insert := func(id, binding, text string, sentAgo time.Duration) {
		t.Helper()
		if err := database.DB.Exec(
			`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, sent_at, sent_text, created_at)
			 VALUES (?, 'u1', ?, 'daily_nudge', ?, ?, ?, ?)`,
			id, binding, time.Now().Add(-sentAgo), time.Now().Add(-sentAgo), text, time.Now().Add(-sentAgo),
		).Error; err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	insert("f-yesterday", "i1", "Como correu a caminhada de ontem?", 24*time.Hour)
	insert("f-old", "i1", "Ainda pensas na sessão de visão?", 3*24*time.Hour)
	// Outside the 14-day window: stale enough that repeating it is fine.
	insert("f-ancient", "i1", "Mensagem muito antiga", 20*24*time.Hour)
	// Another user's binding must never leak in.
	insert("f-other", "i2", "Mensagem de outro utilizador", time.Hour)
	// Failed / still-pending rows have no sent_at or no sent_text and are skipped.
	database.DB.Exec(`INSERT INTO channel_follow_ups (id, user_id, binding_id, kind, scheduled_at, failed_at, sent_text, created_at)
		VALUES ('f-failed', 'u1', 'i1', 'daily_nudge', ?, ?, 'nunca chegou', ?)`,
		time.Now(), time.Now(), time.Now())

	block := s.recentProactiveBlock("i1")
	if !strings.Contains(block, "Como correu a caminhada de ontem?") || !strings.Contains(block, "Ainda pensas na sessão de visão?") {
		t.Errorf("block is missing sent messages:\n%s", block)
	}
	for _, absent := range []string{"Mensagem muito antiga", "Mensagem de outro utilizador", "nunca chegou"} {
		if strings.Contains(block, absent) {
			t.Errorf("block leaked %q:\n%s", absent, block)
		}
	}
	// Newest first: yesterday's message before the older one.
	if strings.Index(block, "caminhada") > strings.Index(block, "sessão de visão") {
		t.Errorf("messages are not newest-first:\n%s", block)
	}

	// The cap: with more history than the limit, only the newest N appear.
	for i := 0; i < recentProactiveLimit+3; i++ {
		insert(fmt.Sprintf("f-bulk-%d", i), "i1", fmt.Sprintf("bulk-%d", i), time.Duration(i+1)*time.Hour)
	}
	block = s.recentProactiveBlock("i1")
	if n := strings.Count(block, "bulk-"); n != recentProactiveLimit {
		t.Errorf("block holds %d bulk messages, want %d", n, recentProactiveLimit)
	}
}
