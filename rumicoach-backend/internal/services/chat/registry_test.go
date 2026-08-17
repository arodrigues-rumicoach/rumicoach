package chat

import (
	"testing"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

func TestSessionRegistry(t *testing.T) {
	cases := []struct {
		typ  api.SessionType
		name string
	}{
		{api.SessionTypeOnboarding, "onboarding"},
		{api.SessionTypeSessionVision, "vision"},
		{api.SessionTypeSessionMovement, "movement"},
		{api.SessionTypeSessionValues, "values"},
		{api.SessionTypeSessionEnergy, "energy"},
		{api.SessionTypeSessionDecisions, "decisions"},
		{api.SessionTypeSessionBeliefs, "beliefs"},
		{api.SessionTypeCheckin, "checkin"},
	}
	for _, c := range cases {
		sess, ok := sessions.Get(c.typ)
		if !ok {
			t.Errorf("session %q not registered", c.typ)
			continue
		}
		if sess.Name() != c.name {
			t.Errorf("session %q has name %q, want %q", c.typ, sess.Name(), c.name)
		}
		if sess.Type() != c.typ {
			t.Errorf("session %q reports type %q", c.name, sess.Type())
		}
		// Every registered session must produce a non-empty instructions prompt and persona.
		// The daily check-in builds its prompt from a DB frequency query, so skip it here.
		if c.typ != api.SessionTypeCheckin {
			if sess.Instructions(sess.InitialState(), session.Context{FirstName: "Armando"}) == "" {
				t.Errorf("session %q returned empty instructions", c.name)
			}
		}
		if sess.SystemPersona(session.Voice{CoachGender: "female", MentorDesc: "motherly", ToneDesc: "x"}) == "" {
			t.Errorf("session %q returned empty persona", c.name)
		}
	}

	// An unregistered type resolves to nothing.
	if _, ok := sessions.Get(api.SessionType("session_nonexistent")); ok {
		t.Error("an unknown session type should not be registered")
	}
}
