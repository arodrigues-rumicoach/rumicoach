package chat

import (
	"testing"
	"time"
)

// QA's example: a user with an exam tomorrow morning must not get "how did it go?" the
// night before. Timing is a coaching decision, so the model gives an absolute LOCAL
// moment — hours-from-now cannot express "the morning of the exam" reliably, because the
// model has no exact anchor for when the session ended.
func TestResolveNotificationTime(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	now := time.Now().In(lisbon)

	// An absolute local moment in the future is honoured to the minute.
	target := now.Add(20 * time.Hour)
	if target.Hour() >= notificationQuietStart || target.Hour() < notificationQuietEnd {
		target = target.Add(10 * time.Hour) // keep the fixture out of quiet hours
	}
	got := resolveNotificationTime(target.Format("2006-01-02 15:04"), 0, lisbon)
	if got.IsZero() {
		t.Fatal("a future local moment must be scheduled")
	}
	if got.In(lisbon).Format("2006-01-02 15:04") != target.Format("2006-01-02 15:04") {
		t.Errorf("scheduled %s, want %s", got.In(lisbon).Format("2006-01-02 15:04"), target.Format("2006-01-02 15:04"))
	}

	// A moment already gone is dropped, not sent late — this is the exam case in
	// reverse: "good luck this morning" has no value once the morning has passed.
	if got := resolveNotificationTime(now.Add(-3*time.Hour).Format("2006-01-02 15:04"), 0, lisbon); !got.IsZero() {
		t.Errorf("a past moment must be dropped, got %s", got)
	}

	// ISO is accepted too — models emit it whatever the schema asks for.
	iso := now.Add(30 * time.Hour)
	if got := resolveNotificationTime(iso.Format("2006-01-02T15:04"), 0, lisbon); got.IsZero() {
		t.Error("the ISO form should parse")
	}

	// delay_hours still works when no moment matters.
	if got := resolveNotificationTime("", 5, lisbon); got.IsZero() {
		t.Error("delay_hours should still schedule when no moment is given")
	}
	// Neither given: dropped rather than guessed.
	if got := resolveNotificationTime("", 0, lisbon); !got.IsZero() {
		t.Error("with no timing at all the notification must be dropped")
	}
	// Garbage is dropped rather than silently becoming "now".
	if got := resolveNotificationTime("tomorrow morning", 0, lisbon); !got.IsZero() {
		t.Error("an unparseable time must not fall through to an arbitrary moment")
	}
}

// Waking someone at 3am undoes far more than the message delivers, and the model cannot
// be trusted never to produce one.
func TestQuietHours(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Skip("tzdata unavailable")
	}

	cases := []struct {
		name     string
		at       time.Time
		wantHour int
		wantDay  int
	}{
		{"3am moves to 7am the same day", time.Date(2026, 8, 5, 3, 0, 0, 0, lisbon), 7, 5},
		{"6:59 moves to 7am", time.Date(2026, 8, 5, 6, 59, 0, 0, lisbon), 7, 5},
		{"23:30 moves to 7am NEXT day", time.Date(2026, 8, 5, 23, 30, 0, 0, lisbon), 7, 6},
		{"22:00 exactly is already quiet", time.Date(2026, 8, 5, 22, 0, 0, 0, lisbon), 7, 6},
		{"9am is left alone", time.Date(2026, 8, 5, 9, 0, 0, 0, lisbon), 9, 5},
		{"21:59 is left alone", time.Date(2026, 8, 5, 21, 59, 0, 0, lisbon), 21, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shiftOutOfQuietHours(c.at, lisbon).In(lisbon)
			if got.Hour() != c.wantHour || got.Day() != c.wantDay {
				t.Errorf("got %s, want day %d at %02d:00", got.Format("2006-01-02 15:04"), c.wantDay, c.wantHour)
			}
			// Never move a message earlier — that could drag it before the event it
			// was written for.
			if got.Before(c.at) {
				t.Errorf("quiet hours moved the message backwards: %s -> %s", c.at, got)
			}
		})
	}
}
