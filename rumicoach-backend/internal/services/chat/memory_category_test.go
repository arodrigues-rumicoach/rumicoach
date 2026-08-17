package chat

import (
	"strings"
	"testing"

	"github.com/rumi/rumi-be/internal/services/chat/session/checkin"
)

// The memory category set is closed. Nothing validated it before, so categories the model
// invented were stored verbatim (QA: 'goals', 'vision') — and an insight filed under a
// made-up category vanishes from the session-summary panel and the profile insight count,
// both of which query category = 'insight'.
func TestNormalizeMemoryCategory(t *testing.T) {
	for _, in := range []string{"insight", "Insight", "  values  ", "OBSTACLES"} {
		got, ok := normalizeMemoryCategory(in)
		if !ok {
			t.Errorf("%q must be accepted", in)
			continue
		}
		if got != strings.ToLower(strings.TrimSpace(in)) {
			t.Errorf("%q normalized to %q", in, got)
		}
	}

	// The two the model actually invented in QA, plus an empty value.
	for _, in := range []string{"goals", "vision", "wheel_of_life", ""} {
		if _, ok := normalizeMemoryCategory(in); ok {
			t.Errorf("%q must be rejected — it is not an api.MemoryCategory", in)
		}
	}
}

// A check-in is a single state with no next stage, so complete_current_task can only ever
// fail there. Declaring it invited the model to reach for it to "close" the session and
// collect two consecutive tool errors instead (QA).
func TestCheckinDoesNotDeclareCompleteCurrentTask(t *testing.T) {
	for name, tools := range map[string][]string{
		"daily": checkin.NewDaily().ToolNames(""),
	} {
		for _, tool := range tools {
			if tool == "complete_current_task" {
				t.Errorf("%s check-in must not declare complete_current_task: it has no transition to make", name)
			}
		}
		if len(tools) == 0 {
			t.Errorf("%s check-in declared no tools at all", name)
		}
	}
}
