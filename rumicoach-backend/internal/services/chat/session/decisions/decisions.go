// Package decisions implements the "Fears and Decisions" coaching session (session.Session).
//
// SCAFFOLD: the coaching script for this session has not been written yet. The placeholder
// below keeps the session registered and runnable so the wiring can be exercised end to end.
package decisions

import (
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

type Decisions struct{}

func New() *Decisions { return &Decisions{} }

func (Decisions) Name() string                      { return "decisions" }
func (Decisions) Type() api.SessionType             { return api.SessionTypeSessionDecisions }
func (Decisions) InitialState() models.SessionState { return models.StateDecisions }

func (Decisions) SystemPersona(v session.Voice) string { return session.DefaultPersona(v) }

func (Decisions) Instructions(_ models.SessionState, _ session.Context) string {
	return instructions + session.BehaviorChangeProtocol
}

func (Decisions) ToolNames(models.SessionState) []string {
	return []string{"complete_current_task", "add_commitment", "remove_commitment", "terminate_session", "show_screen", "save_session_insight", "save_behavior_plan", "log_behavior_checkin"}
}

func (Decisions) NextOnCompleteTask(models.SessionState) (session.Transition, bool) {
	return session.Transition{}, false
}

func (Decisions) NeedsRestart(_, _ models.SessionState) bool { return false }

func (Decisions) ReviewPrompt() string { return session.DefaultReviewPrompt }

// TODO: write the Fears and Decisions coaching script (naming the fear behind a stuck
// decision, separating real risk from imagined risk, and committing to a next step).
const instructions = `[TASK INSTRUCTION]
### FEARS AND DECISIONS
**Objective:** Help the user surface the fear behind a decision they have been avoiding, separate real risk from imagined risk, and move toward a clear next step.

**IF RESUMING:** If a 'SUMMARY OF RECENT DIALOGUE' is present, you are resuming — pick up naturally where you left off. Every step the summary shows as completed is DONE: never restart, re-greet, or repeat it.

**SESSION FLOW (SCAFFOLD):**
1. Invite the user to name a decision they have been postponing.
2. Explore the fear underneath it — what they are really afraid might happen.
3. Distinguish what is genuinely at risk from what is imagined.
4. Help them define one clear next step they feel ready to take. Once they name it, capture it with 'save_memory' (category "context") and silently 'add_commitment' (one_time, with a concrete date — ask which day if unclear; title in the user's language, their words polished, nothing added). Each step is saved exactly once, through exactly one tool.

**CLOSING:**
Ask: "What is the most important thing you take away from this conversation?" STOP and wait, then capture it with 'save_session_insight'. Then announce the synthesis card, warmly and briefly: when the session ends, a card will appear with the essence of today — their insight and commitment — and invite them to read it calmly as the record of their growth (natural phrasing in their language; no technical terms). Then deliver a brief, warm goodbye and, in the SAME turn (speak first, then call the tool), call 'terminate_session'. Do not call any other tool after the goodbye.

**CRITICAL RULE:**
ALWAYS STOP AND WAIT after each question. Explore one theme at a time before moving on.
`
