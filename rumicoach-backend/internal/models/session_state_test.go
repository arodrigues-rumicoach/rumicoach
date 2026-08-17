package models

import "testing"

// The onboarding/Vision split moved five states off the ONBOARDING_ prefix. Several
// behaviours key off these predicates — balance exemption, journey routing, the admin
// funnel — so a state landing in the wrong family is a silent, expensive bug: a user
// billed for a free session, or bounced back through onboarding forever.
func TestSessionStateFamilies(t *testing.T) {
	cases := []struct {
		state      SessionState
		onboarding bool
		vision     bool
	}{
		{StateOnboardingIntro, true, false},
		{StateLegacyOnboarding, true, false},
		{StateVisionIdealLife, false, true},
		{StateVisionWheelOfLife, false, true},
		{StateVisionMetaphor, false, true},
		{StateVisionEmotionalClosing, false, true},
		{StateVisionEndingSession, false, true},
		{StateCheckin, false, false},
		{StateMovement, false, false},
		{StateValues, false, false},
		{StateEndingSession, false, false},
	}
	for _, c := range cases {
		if got := c.state.IsOnboarding(); got != c.onboarding {
			t.Errorf("%s.IsOnboarding() = %v, want %v", c.state, got, c.onboarding)
		}
		if got := c.state.IsVision(); got != c.vision {
			t.Errorf("%s.IsVision() = %v, want %v", c.state, got, c.vision)
		}
	}
}

// StateValues is "VALUES" — it must not be swept into the Vision family by a loose
// prefix match, which would route a Values session to the Vision prompts.
func TestValuesStateIsNotVision(t *testing.T) {
	if StateValues.IsVision() {
		t.Error("StateValues must not match the VISION_ prefix")
	}
}

// The billing exemption used to be decided here, from users.state. It is now counted
// from sessions on record — see balance.WithinFreeAllowance and its tests. Nothing in
// this file should grow a "is this free?" predicate again: state defaults to
// VISION_IDEAL_LIFE on every new account, so any rule of that shape starts out
// exempting everybody and only stops if the Vision session is finished cleanly.

// The closing predicates follow the phase, not the session family: both the Vision
// session's own closing states and the generic ones used by later sessions must match.
func TestClosingPredicates(t *testing.T) {
	for _, s := range []SessionState{StateVisionEmotionalClosing, StateEmotionalClosing} {
		if !s.IsEmotionalClosing() {
			t.Errorf("%s should be an emotional closing state", s)
		}
	}
	for _, s := range []SessionState{StateVisionEndingSession, StateEndingSession} {
		if !s.IsEndingSession() {
			t.Errorf("%s should be an ending state", s)
		}
	}
	if StateVisionIdealLife.IsEmotionalClosing() || StateVisionIdealLife.IsEndingSession() {
		t.Error("the ideal-life state is neither a closing nor an ending state")
	}
}
