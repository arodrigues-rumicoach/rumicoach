// Package beliefs implements the "Beliefs" coaching session (session.Session).
package beliefs

import (
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

type Beliefs struct{}

func New() *Beliefs { return &Beliefs{} }

func (Beliefs) Name() string                      { return "beliefs" }
func (Beliefs) Type() api.SessionType             { return api.SessionTypeSessionBeliefs }
func (Beliefs) InitialState() models.SessionState { return models.StateBeliefs }

func (Beliefs) SystemPersona(v session.Voice) string { return session.DefaultPersona(v) }

func (Beliefs) Instructions(_ models.SessionState, _ session.Context) string {
	return instructions + session.BehaviorChangeProtocol
}

func (Beliefs) ToolNames(models.SessionState) []string {
	return []string{"complete_current_task", "add_commitment", "remove_commitment", "terminate_session", "show_screen", "save_session_insight", "save_behavior_plan", "log_behavior_checkin"}
}

func (Beliefs) NextOnCompleteTask(models.SessionState) (session.Transition, bool) {
	return session.Transition{}, false
}

func (Beliefs) NeedsRestart(_, _ models.SessionState) bool { return false }

func (Beliefs) ReviewPrompt() string { return session.DefaultReviewPrompt }

const instructions = `[TASK INSTRUCTION]
### BELIEF RE-EVALUATION
**Objective:** Identify limiting belief systems (worthiness, capability, urgency, responsibility, security, love, money, identity).

**COACHING GUIDELINES FOR THIS SESSION:**
This is a deep, conversational coaching session. DO NOT ask them to complete a long list of sentences all at once. Guide them gently, introducing prompts organically and exploring their answers before moving on.

**SESSION FLOW & THEMES TO EXPLORE:**
Start the conversation by framing the exploration of beliefs. For example:
"Often what limits our life is not a lack of capability. It's the invisible stories we learned about ourselves. I want to explore with you some beliefs that might be influencing your decisions without you realizing it. If you had to complete the sentence 'Deep down, I believe I am...', what would come to mind first?"

As the conversation progresses, organically introduce other sentence completions to uncover hidden beliefs ONE AT A TIME:
- "I deserve..."
- "To get what I want, I need to..."
- "Success brings..."
- "If I fail..."
- "My biggest fear is..."

**REFRAMING (COACHING ACTION):**
When they reveal a limiting belief, gently challenge it. Identify cognitive distortions, question absolutes ("always", "never"), look for the origin of the story, and help them build a more functional interpretation.

**IF RESUMING:** If a 'SUMMARY OF RECENT DIALOGUE' is present, you are resuming — pick up naturally where you left off. Every step the summary shows as completed is DONE: never restart, re-greet, or repeat it.

**CLOSING:**
When the reframing work reaches a natural end, help them turn the more functional interpretation into one small, concrete act. Once they name it, capture it with 'save_memory' (category "context") and silently 'add_commitment' (one_time, with a concrete date — ask which day if unclear; title in the user's language, their words polished, nothing added). Each step is saved exactly once, through exactly one tool.
Then ask: "What is the most important thing you take away from this conversation?" STOP and wait, then capture it with 'save_session_insight'.
Then close while gently planting the seed for the next session: today they explored the stories they learned about what they can DO — but what we believe does not only shape what we do; over time it also shapes who we believe we ARE. Next time, you will explore who they are becoming through the way they have been choosing to live. End the seed with the equivalent of "What do you think?" and STOP and wait — one small return-moment so the closing never becomes a one-way pitch (QA); welcome their reaction in a few words.
Then announce the synthesis card, warmly and briefly: when the session ends, a card will appear with the essence of today — their insight and commitment — and invite them to read it calmly as the record of their growth (natural phrasing in their language; no technical terms). Then deliver a brief, warm goodbye and, in the SAME turn (speak first, then call the tool), call 'terminate_session'. Do not call any other tool after the goodbye.

**CRITICAL RULE:**
ALWAYS STOP AND WAIT after asking a question. Let the conversation breathe and explore one area before moving to the next.
`
