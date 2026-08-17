package companion

import (
	"testing"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
)

// Both chat rate limits used to be COUNTs over channel_messages, which meant a user who
// cleared their conversation could be messaged again on the spot and got a fresh daily
// allowance for the trouble. The state moved onto the binding — but only if the send and
// receive paths actually write it. The purge-side guarantee is tested in the handlers
// package; this is the other half, and without it the whole thing is decorative.
func TestRecordOutboundStampsTheBinding(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	long := time.Now().Add(-48 * time.Hour)
	addIntegration(t, "i1", &long)

	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}
	if !integration.MayReachOutAfter(proactiveQuietPeriod) {
		t.Fatal("a binding we have never written to must be reachable")
	}

	s.recordOutbound(&integration, models.ChannelMessageTypeText, "olá", nil, models.ChannelMessageSent, 7, Spend{})

	// In memory...
	if integration.MayReachOutAfter(proactiveQuietPeriod) {
		t.Error("sending must open the quiet period on the in-memory binding")
	}
	// ...and persisted, which is what the next dispatcher tick will read.
	var reloaded models.Integration
	if err := database.DB.First(&reloaded, "id = 'i1'").Error; err != nil {
		t.Fatalf("reload integration: %v", err)
	}
	if reloaded.LastOutboundAt == nil {
		t.Fatal("last_outbound_at was not persisted; the quiet period would reset on every restart")
	}
	if reloaded.MayReachOutAfter(proactiveQuietPeriod) {
		t.Error("the persisted binding should be inside the quiet period")
	}
}

// The counter rolls over on the UTC calendar day, and is compared as a date rather than an
// instant: the column is a DATE and neither driver guarantees an exact midnight coming back.
func TestRollDailyInbound(t *testing.T) {
	now := time.Date(2026, 8, 3, 14, 30, 0, 0, time.UTC)
	i := &models.Integration{}

	if got := i.RollDailyInbound(now); got != 1 {
		t.Errorf("first message of the day = %d, want 1", got)
	}
	if got := i.RollDailyInbound(now.Add(time.Hour)); got != 2 {
		t.Errorf("second message = %d, want 2", got)
	}

	// A stored date that is not today restarts the count, however it came back from the
	// driver — here with a time component attached.
	odd := time.Date(2026, 8, 2, 23, 59, 59, 0, time.UTC)
	i.DailyInboundDate = &odd
	if got := i.RollDailyInbound(now); got != 1 {
		t.Errorf("a new day should restart the count, got %d", got)
	}

	// Same calendar day expressed in another zone is still the same day.
	sameDayElsewhere := now.In(time.FixedZone("UTC+9", 9*3600))
	if got := i.RollDailyInbound(sameDayElsewhere); got != 2 {
		t.Errorf("the count must key off the UTC day, got %d", got)
	}
}

// The cap is read off the binding, so clearing the message log cannot buy a fresh
// allowance. It also has to keep firing the notice exactly once, on the message that
// crosses the line — not on every one after it.
func TestDailyCapReadsTheBinding(t *testing.T) {
	prev := config.AppConfig
	config.AppConfig = &config.Config{CompanionDailyMessageCap: 3}
	t.Cleanup(func() { config.AppConfig = prev })

	s := newToolService()
	i := &models.Integration{}

	for n := 1; n <= 3; n++ {
		i.DailyInboundCount = n
		if capped, _ := s.dailyCapState(i); capped {
			t.Errorf("message %d of 3 must not be capped", n)
		}
	}

	i.DailyInboundCount = 4
	capped, notify := s.dailyCapState(i)
	if !capped || !notify {
		t.Errorf("the message crossing the line should be capped and notify, got %v/%v", capped, notify)
	}

	i.DailyInboundCount = 5
	capped, notify = s.dailyCapState(i)
	if !capped || notify {
		t.Errorf("later messages stay capped but must not repeat the notice, got %v/%v", capped, notify)
	}
}
