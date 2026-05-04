package builtin_test

import (
	"testing"

	_ "github.com/kaeawc/datalint/internal/rules/builtin"
	"github.com/kaeawc/datalint/internal/testutil"
)

// These tests exercise the end-to-end suppression path: rules emit
// findings as usual, the pipeline post-filters them via the
// suppression package using markers in the source files.

func TestSuppression_PythonComment(t *testing.T) {
	// Fixture has random.shuffle on lines 4 and 5; line 4 has a
	// disable comment for random-seed-not-set. With the suppression
	// filter, only line 5's finding survives.
	path := testutil.Fixture(t, "suppression/python-disable.py")
	got := findingsForRule(t, path, "random-seed-not-set")

	if len(got) != 1 {
		t.Fatalf("expected 1 finding (line 5), got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Line != 5 {
		t.Errorf("survivor line = %d, want 5 (line 4 should be suppressed)", got[0].Location.Line)
	}
}

func TestSuppression_JSONLRowDisable(t *testing.T) {
	// Rows 2 and 3 both have an assistant-after-assistant role
	// inversion. Row 2 carries _datalint_disable for role-inversion,
	// so only row 3's finding should survive.
	path := testutil.Fixture(t, "suppression/jsonl-disable.jsonl")
	got := findingsForRule(t, path, "role-inversion")

	if len(got) != 1 {
		t.Fatalf("expected 1 finding (row 3), got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 3 {
		t.Errorf("survivor row = %d, want 3 (row 2 should be suppressed)", got[0].Location.Row)
	}
}
