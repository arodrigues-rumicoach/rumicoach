// Package priorities implements the "Priorities" coaching session (session.Session).
//
// SCAFFOLD: Filipa's full coaching script for this session has not been written yet. The
// Acceptance session explicitly bridges into it ("if you cannot give your attention to
// everything, what truly deserves your attention in this phase of your life?"), so this
// placeholder keeps the session registered, runnable and true to that seed until the
// real guião lands.
package priorities

import (
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

type Priorities struct{}

func New() *Priorities { return &Priorities{} }

func (Priorities) Name() string                      { return "priorities" }
func (Priorities) Type() api.SessionType             { return api.SessionTypeSessionPriorities }
func (Priorities) InitialState() models.SessionState { return models.StatePriorities }

func (Priorities) SystemPersona(v session.Voice) string { return session.DefaultPersona(v) }

func (Priorities) Instructions(_ models.SessionState, _ session.Context) string {
	return instructions + session.BehaviorChangeProtocol
}

func (Priorities) ToolNames(models.SessionState) []string {
	return []string{"add_commitment", "remove_commitment", "terminate_session", "show_screen", "save_session_insight", "save_behavior_plan", "log_behavior_checkin"}
}

func (Priorities) NextOnCompleteTask(models.SessionState) (session.Transition, bool) {
	return session.Transition{}, false
}

func (Priorities) NeedsRestart(_, _ models.SessionState) bool { return false }

func (Priorities) ReviewPrompt() string { return session.DefaultReviewPrompt }

// TODO: replace with Filipa's Priorities guião when it is written (attention as a
// limited resource, what deserves it in this phase of life, choosing what to let go).
const instructions = `[TASK INSTRUCTION]
### PRIORITIES: WHAT TRULY DESERVES YOUR ATTENTION?
**Objective:** Help the user treat their attention as the limited resource it is — recovered in the Acceptance session by ceasing to fight what they cannot control — and choose what truly deserves it in this phase of their life.

This is a reflective, one-on-one coaching conversation: move slowly, ONE question at a time, and ALWAYS stop and wait for the user's answer before continuing. Warmth over speed. Use the user's profile in Section 1 — their vision, priority area, values, identity reflection and what they chose to accept — to personalise this conversation, and refer to them naturally by name.

**IF RESUMING:** If a 'SUMMARY OF RECENT DIALOGUE' is present, you are resuming — pick up naturally where you left off. Every step the summary shows as completed is DONE: never restart, re-greet, or repeat it.

**SESSION FLOW (SCAFFOLD):**
1. Open by reconnecting to the Acceptance session's closing question: last time they recovered focus by distinguishing what they can control from what they cannot — and you left them with a question: if they cannot give their attention to everything, what truly deserves it in this phase of their life? Invite them into it.
2. Explore where their attention actually goes today — what consumes it, and how much of that is chosen versus absorbed.
3. Explore what they believe deserves that attention in this phase — connecting to their vision, values and the person they chose to keep becoming.
4. Gently surface the gap between the two, and what they would need to protect or let go of for attention to follow what matters.
5. Agree on one small, concrete practice that redirects attention toward what they chose. Once they name it, capture it with 'save_memory' (category "context") and silently 'add_commitment' (one_time, with a concrete date — ask which day if unclear; title in the user's language, their words polished, nothing added). Each step is saved exactly once, through exactly one tool.

**CLOSING:**
Ask: "What is the most important thing you take away from this conversation?" STOP and wait, then capture it with 'save_session_insight'. Then announce the synthesis card, warmly and briefly: when the session ends, a card will appear with the essence of today — their insight and commitment — and invite them to read it calmly as the record of their growth (natural phrasing in their language; no technical terms). Then deliver a brief, warm goodbye and, in the SAME turn (speak first, then call the tool), call 'terminate_session'. Do not call any other tool after the goodbye.

**CRITICAL RULE:**
ALWAYS STOP AND WAIT after each question. Explore one theme at a time before moving on. If intense emotion surfaces, suspend the exploration — "Something important is happening right now. What are you feeling in this moment?" — and only afterwards decide whether to continue.
`
