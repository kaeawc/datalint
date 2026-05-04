package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const optionalFieldRuleID = "optional-field-required-by-downstream"

func TestOptionalFieldRequired_PartialMissing(t *testing.T) {
	// 6 rows. "name" present in 5/6 = 83.3%, above default 0.8
	// threshold and below 1.0 → flag at row 5 (the first missing).
	// "id" present in 6/6 = 100% → not flagged.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)

	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 5 {
		t.Errorf("row = %d, want 5 (first row missing 'name')", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, `"name"`) {
		t.Errorf("message should name the field: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "5/6") {
		t.Errorf("message should report the ratio: %q", got[0].Message)
	}
}

func TestOptionalFieldRequired_AllPresent(t *testing.T) {
	// Every row has every field → every ratio is 1.0 → no finding.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-stable.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_HalfMissingBelowThreshold(t *testing.T) {
	// "name" present in 3/6 = 50%, below 0.8 threshold → no flag.
	// The rule deliberately excludes mostly-absent fields; that's a
	// different signal than "almost-always present, occasionally
	// missing".
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-half-missing.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (50%% presence is below threshold), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_TooShortToConclude(t *testing.T) {
	// 4 rows, below default min_rows=5 → rule stays silent even
	// though "name" is missing in 1 of 4.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-too-short.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (below min_rows), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_ConfigOverride(t *testing.T) {
	// With a tightened threshold of 0.9, "name" at 83.3% no longer
	// trips the rule. Verifies ctx.Settings.Float plumbs through.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"min_presence_ratio": 0.9,
	}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == optionalFieldRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 findings under raised threshold, got %d: %s",
			len(got), joinMessages(got))
	}
}
