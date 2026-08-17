package notification

import (
	"testing"
	"time"
)

// A session end schedules several notifications at once. Delivered as a batch they read
// as a queue being flushed, not as someone thinking of you — so anything inside the
// quiet window is deferred, never dropped.
func TestDeferUntil(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	cases := []struct {
		name      string
		lastSent  *time.Time
		wantDefer bool
		wantAt    time.Time
	}{
		{"never had one — send now", nil, false, time.Time{}},
		{"zero timestamp is not a delivery", &time.Time{}, false, time.Time{}},
		{"one an hour ago — wait out the gap", at(-time.Hour), true, now.Add(-time.Hour).Add(minGapBetweenNotifications)},
		{"one just now — wait almost the full gap", at(-time.Minute), true, now.Add(-time.Minute).Add(minGapBetweenNotifications)},
		{"exactly at the boundary — free to send", at(-minGapBetweenNotifications), false, time.Time{}},
		{"long ago — free to send", at(-48 * time.Hour), false, time.Time{}},
		// Clock skew must not stall a user's notifications indefinitely.
		{"timestamp in the future — send rather than stall", at(time.Hour), false, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotAt, gotDefer := deferUntil(c.lastSent, now)
			if gotDefer != c.wantDefer {
				t.Fatalf("defer = %v, want %v", gotDefer, c.wantDefer)
			}
			if c.wantDefer && !gotAt.Equal(c.wantAt) {
				t.Errorf("retry at %s, want %s", gotAt, c.wantAt)
			}
			// Deferring must always move it FORWARD, never into the past where it
			// would be picked up again on the very next tick.
			if c.wantDefer && !gotAt.After(now.Add(-minGapBetweenNotifications)) {
				t.Errorf("retry time %s does not move the notification forward", gotAt)
			}
		})
	}
}

func TestSpacingWindowIsMeaningful(t *testing.T) {
	if minGapBetweenNotifications < time.Hour {
		t.Errorf("a gap of %v is too short to stop a burst reading as automation", minGapBetweenNotifications)
	}
}
