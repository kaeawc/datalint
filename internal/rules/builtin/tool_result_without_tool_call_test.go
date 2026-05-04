package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const toolResultWithoutToolCallRuleID = "tool-result-without-tool-call"

func TestToolResultWithoutToolCall_Positive(t *testing.T) {
	path := testutil.Fixture(t, "tool-result-without-tool-call/positive.jsonl")
	got := findingsForRule(t, path, toolResultWithoutToolCallRuleID)

	// Row 1: tool message with no assistant before it at all.
	// Row 2: assistant exists but never called a tool, then tool
	// shows up.
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}
	rows := []int{got[0].Location.Row, got[1].Location.Row}
	wantRows := []int{1, 2}
	if rows[0] != wantRows[0] || rows[1] != wantRows[1] {
		t.Errorf("rows = %v, want %v", rows, wantRows)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "no preceding assistant message with tool_calls") {
			t.Errorf("message phrasing unexpected: %q", f.Message)
		}
	}
}

func TestToolResultWithoutToolCall_Negative(t *testing.T) {
	// Row 1: properly paired tool_calls then tool. Row 2: no tools
	// at all. Row 3: tool_calls but no tool result (orphan
	// declaration; out of scope for this rule).
	path := testutil.Fixture(t, "tool-result-without-tool-call/negative.jsonl")
	got := findingsForRule(t, path, toolResultWithoutToolCallRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestToolResultWithoutToolCall_NoMessagesField(t *testing.T) {
	path := testutil.Fixture(t, "role-inversion/no-messages.jsonl")
	got := findingsForRule(t, path, toolResultWithoutToolCallRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}
