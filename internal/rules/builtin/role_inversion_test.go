package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const roleInversionRuleID = "role-inversion"

func TestRoleInversion_Positive(t *testing.T) {
	path := testutil.Fixture(t, "role-inversion/positive.jsonl")
	got := findingsForRule(t, path, roleInversionRuleID)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}
	rows := []int{got[0].Location.Row, got[1].Location.Row}
	wantRows := []int{2, 4}
	if rows[0] != wantRows[0] || rows[1] != wantRows[1] {
		t.Errorf("rows = %v, want %v", rows, wantRows)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "assistant") {
			t.Errorf("message should mention assistant: %q", f.Message)
		}
	}
}

func TestRoleInversion_Negative(t *testing.T) {
	path := testutil.Fixture(t, "role-inversion/negative.jsonl")
	got := findingsForRule(t, path, roleInversionRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestRoleInversion_NoMessagesField(t *testing.T) {
	// Rows without a messages array — including non-chat schemas — should
	// be ignored, not produce errors.
	path := testutil.Fixture(t, "role-inversion/no-messages.jsonl")
	got := findingsForRule(t, path, roleInversionRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on rows without messages, got %d: %s",
			len(got), joinMessages(got))
	}
}
