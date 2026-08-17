package checkin

import (
	"fmt"
	"strings"
)

// framework is a structured exercise the daily check-in can offer when it genuinely fits the
// user's state (e.g. the Eisenhower Matrix when they feel overwhelmed). Adding a new framework
// is a matter of appending another entry to supportedFrameworks; the check-in prompt renders
// them all in its SUPPORTED FRAMEWORKS section.
type framework struct {
	Name      string // human-facing name, e.g. "Eisenhower Matrix"
	WhenToUse string // the signal that should make the coach offer it
	HowToRun  string // the step-by-step coaching script, including which tools to call
}

// supportedFrameworks are the frameworks available during a daily check-in.
var supportedFrameworks = []framework{eisenhowerMatrixFramework}

var eisenhowerMatrixFramework = framework{
	Name:      "Eisenhower Matrix",
	WhenToUse: "the user feels overwhelmed, scattered, or has too many tasks, worries, or projects competing for their attention",
	HowToRun: `Offer it gently first ("Would it help to sort through everything on your plate together?") and only proceed if they agree.
When they agree, your FIRST commitment in that turn — before explaining anything — is to call the 'init_eisenhower_matrix' tool natively: it is the ONLY thing that opens the matrix on the user's screen. A ◆ screen marker does NOT open the matrix and must NEVER be used for it (it opens an unrelated screen and leaves the user staring at the wrong thing). Only after that tool call, guide them through it conversationally:
1. Explain the idea simply: you will take everything on their mind and sort it into four quadrants — Urgent and Important (do now), Important but Not Urgent (schedule), Urgent but Not Important (delegate or shrink), and Neither (let go).
2. Ask them to brain-dump everything taking up mental space — tasks, worries, projects, small chores. STOP and wait; let them empty their head fully before sorting.
3. Work through the items together, a few at a time. For each, agree on the quadrant and briefly reflect on why. As you categorise, call 'update_eisenhower_matrix' with a JSON array of objects like [{"task":"Finish report","quadrant":"urgent_important","reasoning":"Due today"}] so the screen updates live. Valid quadrant values: urgent_important, not_urgent_important, urgent_not_important, not_urgent_not_important.
4. If the user wants to remove an item, call 'delete_eisenhower_matrix_tasks' with the task names. Do NOT try to drop an item by sending a partial list to 'update_eisenhower_matrix' — that tool only adds or updates.
5. STOP and wait after each update — discuss, don't dump everything at once.
6. When the board feels complete, help them step back and see the picture: what truly deserves their focus, what to schedule, and what to release. Close the exercise by agreeing on the ONE thing worth doing first, then return naturally to the conversation.`,
}

// buildFrameworksSection renders the supported frameworks into the check-in prompt. The coach
// offers a framework only when it genuinely helps — it is never forced.
func buildFrameworksSection() string {
	var sb strings.Builder
	sb.WriteString(`**SUPPORTED FRAMEWORKS:**
During this conversation you may offer ONE of the structured frameworks below — but only when it genuinely fits where the user is. Never force a framework; let it emerge from the conversation. When the user accepts, follow its steps and use the tools exactly as described.

`)
	for _, f := range supportedFrameworks {
		fmt.Fprintf(&sb, "--- %s ---\n*Offer when:* %s.\n%s\n\n", f.Name, f.WhenToUse, f.HowToRun)
	}
	return sb.String()
}
