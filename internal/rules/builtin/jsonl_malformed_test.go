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

func TestJSONLMalformed_Positive(t *testing.T) {
	path := testutil.Fixture(t, "jsonl-malformed-line/positive.jsonl")
	findings, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}

	rows := make([]int, 0, len(findings))
	for _, f := range findings {
		if f.RuleID != "jsonl-malformed-line" {
			t.Errorf("unexpected rule id %q", f.RuleID)
		}
		rows = append(rows, f.Location.Row)
	}

	// Fixture has malformed JSON on row 2 and a blank line on row 4.
	wantRows := []int{2, 4}
	if !equalInts(rows, wantRows) {
		t.Fatalf("findings rows = %v, want %v (messages: %s)", rows, wantRows, joinMessages(findings))
	}
}

func TestJSONLMalformed_Negative(t *testing.T) {
	path := testutil.Fixture(t, "jsonl-malformed-line/negative.jsonl")
	findings, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(findings), joinMessages(findings))
	}
}

func TestJSONLMalformed_NonJSONLPathSkipped(t *testing.T) {
	// A .py path should not be classified as JSONL, so the rule must
	// not fire (and must not try to open a file that may not exist).
	findings, err := pipeline.Run([]string{"does-not-exist.py"}, config.Default())
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestJSONLMalformed_DisabledViaConfig(t *testing.T) {
	// Confirm the dispatcher honors Config.Disable: a rule that would
	// fire 2 findings on this fixture must produce 0 when listed in
	// Disable.
	path := testutil.Fixture(t, "jsonl-malformed-line/positive.jsonl")
	cfg := config.Default()
	cfg.Disable = []string{"jsonl-malformed-line"}
	findings, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	for _, f := range findings {
		if f.RuleID == "jsonl-malformed-line" {
			t.Errorf("disabled rule still fired: %+v", f)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinMessages(findings []diag.Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, f.Message)
	}
	return strings.Join(parts, " | ")
}
