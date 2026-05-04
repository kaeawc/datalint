package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const enumDriftRuleID = "enum-drift"

func TestEnumDrift_Positive(t *testing.T) {
	path := testutil.Fixture(t, "enum-drift/positive.jsonl")
	got := findingsForRule(t, path, enumDriftRuleID)

	// "extra-large" first appears at row 6 (post lock-in). Once
	// flagged, the same value shouldn't fire again at row 8.
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 6 {
		t.Errorf("row = %d, want 6", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, `"extra-large"`) {
		t.Errorf("message missing new value: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, `"label"`) {
		t.Errorf("message missing field name: %q", got[0].Message)
	}
}

func TestEnumDrift_Stable(t *testing.T) {
	path := testutil.Fixture(t, "enum-drift/negative-stable.jsonl")
	got := findingsForRule(t, path, enumDriftRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestEnumDrift_TooShortToLockIn(t *testing.T) {
	// Only 4 rows; lock-in needs 5. Even if row 4 is a brand-new
	// value the rule must stay silent because it hasn't decided
	// what the field's enum looks like yet.
	path := testutil.Fixture(t, "enum-drift/negative-short.jsonl")
	got := findingsForRule(t, path, enumDriftRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestEnumDrift_ConfigOverrideRaisesLockIn(t *testing.T) {
	// With lock_in_rows=10 the positive fixture (only 8 rows) never
	// reaches lock-in, so what looked like drift under the default
	// goes silent. Verifies the rule reads from ctx.Settings.
	path := testutil.Fixture(t, "enum-drift/positive.jsonl")
	cfg := config.Default()
	cfg.Rules["enum-drift"] = map[string]any{"lock_in_rows": 10}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == enumDriftRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 findings with raised lock-in, got %d: %s",
			len(got), joinMessages(got))
	}
}
