package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
	"github.com/kaeawc/datalint/internal/testutil"
)

const fieldTypeRuleID = "field-type-mixed-across-rows"

func TestFieldTypeMixed_Positive(t *testing.T) {
	path := testutil.Fixture(t, "field-type-mixed-across-rows/positive.jsonl")
	got := findingsForRule(t, path, fieldTypeRuleID)

	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 3 {
		t.Errorf("row = %d, want 3", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, `"score"`) {
		t.Errorf("message missing field name: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "number dominant") {
		t.Errorf("message should call out number as dominant: %q", got[0].Message)
	}
}

func TestFieldTypeMixed_NullsIgnored(t *testing.T) {
	path := testutil.Fixture(t, "field-type-mixed-across-rows/negative-with-nulls.jsonl")
	got := findingsForRule(t, path, fieldTypeRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (null+typed is fine), got %d: %s", len(got), joinMessages(got))
	}
}

func TestFieldTypeMixed_Negative(t *testing.T) {
	path := testutil.Fixture(t, "field-type-mixed-across-rows/negative.jsonl")
	got := findingsForRule(t, path, fieldTypeRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

// findingsForRule runs the pipeline and returns only findings from
// the given rule, filtering out any from other registered rules.
func findingsForRule(t *testing.T, path, id string) []diag.Finding {
	t.Helper()
	all, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var out []diag.Finding
	for _, f := range all {
		if f.RuleID == id {
			out = append(out, f)
		}
	}
	return out
}
