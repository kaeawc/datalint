package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const privacyPIIRuleID = "privacy-pii-detected"

func TestPrivacyPII_Builtins(t *testing.T) {
	// 6 rows: row 1 clean, rows 2-5 each match exactly one builtin
	// pattern, row 6 clean. Expect 4 findings on rows 2,3,4,5.
	path := testutil.Fixture(t, "privacy-pii-detected/positive.jsonl")
	got := findingsForRule(t, path, privacyPIIRuleID)

	if len(got) != 4 {
		t.Fatalf("expected 4 findings, got %d: %s", len(got), joinMessages(got))
	}
	wantRows := []int{2, 3, 4, 5}
	for i, f := range got {
		if f.Location.Row != wantRows[i] {
			t.Errorf("finding %d row = %d, want %d", i, f.Location.Row, wantRows[i])
		}
	}

	wantPatterns := []string{"email", "us-ssn", "phone", "credit-card"}
	for i, want := range wantPatterns {
		if !strings.Contains(got[i].Message, want+" pattern") {
			t.Errorf("finding %d should cite %q pattern: %q", i, want, got[i].Message)
		}
	}
}

func TestPrivacyPII_CleanRows(t *testing.T) {
	// Negative fixture: harmless prompts and a numeric id field that
	// isn't a string (rule only scans strings).
	path := testutil.Fixture(t, "privacy-pii-detected/negative.jsonl")
	got := findingsForRule(t, path, privacyPIIRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on clean fixture, got %d: %s", len(got), joinMessages(got))
	}
}

func TestPrivacyPII_ExtraPatternsFromConfig(t *testing.T) {
	// Negative fixture is clean against builtins. Adding an
	// extra_pattern that matches the literal "Order numbers like" on
	// row 3 should surface a single finding citing the custom
	// pattern's user-supplied name.
	path := testutil.Fixture(t, "privacy-pii-detected/negative.jsonl")
	cfg := config.Default()
	cfg.Rules[privacyPIIRuleID] = map[string]any{
		"extra_patterns": []any{`order-id=Order numbers like \d+`},
	}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == privacyPIIRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding from extra_patterns, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 3 {
		t.Errorf("row = %d, want 3", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, "order-id pattern") {
		t.Errorf("message should cite custom name 'order-id': %q", got[0].Message)
	}
}
