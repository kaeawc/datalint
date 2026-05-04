package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const systemMidConvRuleID = "system-message-mid-conversation"

func TestSystemMidConversation_Positive(t *testing.T) {
	path := testutil.Fixture(t, "system-message-mid-conversation/positive.jsonl")
	got := findingsForRule(t, path, systemMidConvRuleID)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}
	rows := []int{got[0].Location.Row, got[1].Location.Row}
	wantRows := []int{2, 3}
	if rows[0] != wantRows[0] || rows[1] != wantRows[1] {
		t.Errorf("rows = %v, want %v", rows, wantRows)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "system message at index") {
			t.Errorf("message should call out the index: %q", f.Message)
		}
	}
}

func TestSystemMidConversation_Negative(t *testing.T) {
	// Row 1: system at index 0 (conventional). Row 2: no system. Row
	// 3: system as the only message (still index 0). All clean.
	path := testutil.Fixture(t, "system-message-mid-conversation/negative.jsonl")
	got := findingsForRule(t, path, systemMidConvRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestSystemMidConversation_NoMessagesField(t *testing.T) {
	// Reuses the role-inversion fixture covering rows without a
	// messages array — same code path, same expectation.
	path := testutil.Fixture(t, "role-inversion/no-messages.jsonl")
	got := findingsForRule(t, path, systemMidConvRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on rows without messages, got %d: %s",
			len(got), joinMessages(got))
	}
}
