package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/rules"
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
	"github.com/kaeawc/datalint/internal/testutil"
)

const trainEvalOverlapRuleID = "train-eval-overlap"

func TestTrainEvalOverlap_Positive(t *testing.T) {
	train := testutil.Fixture(t, "train-eval-overlap/train.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-positive.jsonl")

	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Train: []string{train},
		Eval:  []string{eval},
	}, trainEvalOverlapRuleID)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}
	rows := []int{got[0].Location.Row, got[1].Location.Row}
	wantRows := []int{2, 3}
	if rows[0] != wantRows[0] || rows[1] != wantRows[1] {
		t.Errorf("rows = %v, want %v", rows, wantRows)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "train.jsonl") {
			t.Errorf("message should cite the train path: %q", f.Message)
		}
	}
}

func TestTrainEvalOverlap_Clean(t *testing.T) {
	train := testutil.Fixture(t, "train-eval-overlap/train.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-clean.jsonl")

	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Train: []string{train},
		Eval:  []string{eval},
	}, trainEvalOverlapRuleID)

	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestTrainEvalOverlap_NoTrain(t *testing.T) {
	eval := testutil.Fixture(t, "train-eval-overlap/eval-positive.jsonl")
	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Eval: []string{eval},
	}, trainEvalOverlapRuleID)
	if len(got) != 0 {
		t.Fatalf("rule should be a no-op with no train files: %d findings", len(got))
	}
}

func TestTrainEvalOverlap_NoEval(t *testing.T) {
	train := testutil.Fixture(t, "train-eval-overlap/train.jsonl")
	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Train: []string{train},
	}, trainEvalOverlapRuleID)
	if len(got) != 0 {
		t.Fatalf("rule should be a no-op with no eval files: %d findings", len(got))
	}
}

func TestTrainEvalOverlap_NearDupBelowThresholdNoFire(t *testing.T) {
	// near_dup_threshold defaults to 0 → exact match only. Eval and
	// train share content but aren't byte-equal → 0 findings.
	train := testutil.Fixture(t, "train-eval-overlap/train-near-dup.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-near-dup.jsonl")
	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Train: []string{train},
		Eval:  []string{eval},
	}, trainEvalOverlapRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings under exact-match default, got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestTrainEvalOverlap_NearDupAboveThresholdFires(t *testing.T) {
	// With near_dup_threshold=0.4 (well below the ≈0.55-0.90 band
	// the MinHash test calibrated), row 1's eval prompt should be
	// flagged as a near-dup of train row 1.
	train := testutil.Fixture(t, "train-eval-overlap/train-near-dup.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-near-dup.jsonl")
	cfg := config.Default()
	cfg.Rules["train-eval-overlap"] = map[string]any{
		"near_dup_threshold": 0.4,
	}

	all := pipeline.RunCorpus(&rules.CorpusContext{
		Train: []string{train},
		Eval:  []string{eval},
	}, cfg)
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == trainEvalOverlapRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 near-dup finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 1 {
		t.Errorf("row = %d, want 1", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, "near-duplicate") {
		t.Errorf("message should call out near-duplicate: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "similarity") {
		t.Errorf("message should report similarity: %q", got[0].Message)
	}
}

func TestTrainEvalOverlap_ConfigOverrideField(t *testing.T) {
	// Project uses `input` instead of `prompt` for the input field.
	// Without the config override the rule sees no `prompt` keys
	// and emits nothing; with prompt_field=input it should find the
	// shared row.
	train := testutil.Fixture(t, "train-eval-overlap/train-input-field.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-input-field.jsonl")
	cfg := config.Default()
	cfg.Rules["train-eval-overlap"] = map[string]any{"prompt_field": "input"}

	all := pipeline.RunCorpus(&rules.CorpusContext{
		Train: []string{train},
		Eval:  []string{eval},
	}, cfg)
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == trainEvalOverlapRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding under input field, got %d: %s",
			len(got), joinMessages(got))
	}
	if got[0].Location.Row != 1 {
		t.Errorf("eval row = %d, want 1", got[0].Location.Row)
	}
}

func corpusFindingsForRule(t *testing.T, ctx *rules.CorpusContext, id string) []diag.Finding {
	t.Helper()
	all := pipeline.RunCorpus(ctx, config.Default())
	var out []diag.Finding
	for _, f := range all {
		if f.RuleID == id {
			out = append(out, f)
		}
	}
	return out
}
