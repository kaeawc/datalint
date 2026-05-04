package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const unbalancedToolCallIDRuleID = "unbalanced-tool-call-id"

func TestUnbalancedToolCallID_Positive(t *testing.T) {
	path := testutil.Fixture(t, "unbalanced-tool-call-id/positive.jsonl")
	got := findingsForRule(t, path, unbalancedToolCallIDRuleID)

	// Row 1: tool refers to "abc" before any assistant declares it.
	// Row 2: tool refers to "x2" — assistant declared "x1" only.
	// Row 3: forward reference — "future" is declared *after* the
	// tool message; rule must flag the tool message at its index.
	if len(got) != 3 {
		t.Fatalf("expected 3 findings, got %d: %s", len(got), joinMessages(got))
	}

	wantRows := []int{1, 2, 3}
	wantIDs := []string{`"abc"`, `"x2"`, `"future"`}
	for i, f := range got {
		if f.Location.Row != wantRows[i] {
			t.Errorf("finding %d row = %d, want %d", i, f.Location.Row, wantRows[i])
		}
		if !strings.Contains(f.Message, wantIDs[i]) {
			t.Errorf("finding %d message missing %s: %q", i, wantIDs[i], f.Message)
		}
	}
}

func TestUnbalancedToolCallID_Negative(t *testing.T) {
	// Row 1: properly paired single tool call. Row 2: properly paired
	// two tool calls. Row 3: no tools at all. Row 4: tool message
	// missing tool_call_id (schema variant) — tolerated.
	path := testutil.Fixture(t, "unbalanced-tool-call-id/negative.jsonl")
	got := findingsForRule(t, path, unbalancedToolCallIDRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestUnbalancedToolCallID_NoMessagesField(t *testing.T) {
	// Reuses the role-inversion fixture for shared coverage of rows
	// without a messages array.
	path := testutil.Fixture(t, "role-inversion/no-messages.jsonl")
	got := findingsForRule(t, path, unbalancedToolCallIDRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}
