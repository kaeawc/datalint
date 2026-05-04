package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const sysPromptLeakRuleID = "system-prompt-leaks-eval-instructions"

func TestSystemPromptLeaksEval_Positive(t *testing.T) {
	path := testutil.Fixture(t, "system-prompt-leaks-eval-instructions/positive.jsonl")
	got := findingsForRule(t, path, sysPromptLeakRuleID)

	if len(got) != 3 {
		t.Fatalf("expected 3 findings, got %d: %s", len(got), joinMessages(got))
	}
	rows := []int{got[0].Location.Row, got[1].Location.Row, got[2].Location.Row}
	wantRows := []int{1, 2, 3}
	for i, r := range rows {
		if r != wantRows[i] {
			t.Errorf("finding %d row = %d, want %d", i, r, wantRows[i])
		}
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "matches eval-instruction pattern") {
			t.Errorf("message phrasing unexpected: %q", f.Message)
		}
	}
}

func TestSystemPromptLeaksEval_Negative(t *testing.T) {
	path := testutil.Fixture(t, "system-prompt-leaks-eval-instructions/negative.jsonl")
	got := findingsForRule(t, path, sysPromptLeakRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestSystemPromptLeaksEval_NoMessagesField(t *testing.T) {
	path := testutil.Fixture(t, "role-inversion/no-messages.jsonl")
	got := findingsForRule(t, path, sysPromptLeakRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestSystemPromptLeaksEval_ExtraPatternsFromConfig(t *testing.T) {
	// negative.jsonl is clean against the built-in patterns. Add a
	// project-specific pattern via config that matches "creative
	// writing tutor" — row 3's previously-clean prompt now flags.
	path := testutil.Fixture(t, "system-prompt-leaks-eval-instructions/negative.jsonl")
	cfg := config.Default()
	cfg.Rules[sysPromptLeakRuleID] = map[string]any{
		"extra_patterns": []any{`(?i)creative writing tutor`},
	}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == sysPromptLeakRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding from extra_patterns, got %d: %s",
			len(got), joinMessages(got))
	}
	if got[0].Location.Row != 3 {
		t.Errorf("row = %d, want 3", got[0].Location.Row)
	}
}
