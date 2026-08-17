package chat

import (
	"testing"
	"time"
)

// The AI sets the horizon when it agrees a habit with the user. Only recurring
// commitments get one, only valid future dates count, and a bad value must never
// silently end the habit on the day it was created.
func TestIncomingCommitmentEndedAt(t *testing.T) {
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	str := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   incomingCommitment
		want string // "" means nil
	}{
		{"recurring with a future horizon",
			incomingCommitment{Type: "recurring", EndDate: str("2026-08-28")}, "2026-08-28"},
		{"recurring without one runs on",
			incomingCommitment{Type: "recurring"}, ""},
		{"empty string is not a horizon",
			incomingCommitment{Type: "recurring", EndDate: str("  ")}, ""},
		{"one-time ignores it — its date is its end",
			incomingCommitment{Type: "one_time", Date: str("2026-08-01"), EndDate: str("2026-08-28")}, ""},
		{"malformed date is dropped, not applied",
			incomingCommitment{Type: "recurring", EndDate: str("28/08/2026")}, ""},
		{"a past date would end it on creation — dropped",
			incomingCommitment{Type: "recurring", EndDate: str("2026-07-01")}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.in.endedAt(now)
			if c.want == "" {
				if got != nil {
					t.Errorf("endedAt = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("endedAt = nil, want %s", c.want)
			}
			if got.Format("2006-01-02") != c.want {
				t.Errorf("endedAt = %s, want %s", got.Format("2006-01-02"), c.want)
			}
		})
	}
}
