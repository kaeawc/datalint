package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/testutil"
)

const crossDatasetOverlapRuleID = "cross-dataset-overlap"

func TestCrossDatasetOverlap_ThreeWayPairs(t *testing.T) {
	// train.jsonl  (rows 1, 2): "France" / "photosynthesis"
	// eval.jsonl   (rows 1, 2): "photosynthesis" (overlap with train) / "Hamlet"
	// test.jsonl   (rows 1, 2): "France" (overlap with train) / "quantum"
	//
	// Names sort to [eval, test, train]. Each pair (a, b) where a<b
	// indexes a and streams b, anchoring findings on b — the lexically
	// later dataset. Useful for leakage workflows: "remove these from
	// the dataset that came in second."
	//
	//   (eval, test):  no overlap
	//   (eval, train): train row 2 ("photosynthesis") matches eval
	//   (test, train): train row 1 ("France") matches test
	//
	// Both findings end up on train.jsonl, in pair-traversal order.
	train := testutil.Fixture(t, "cross-dataset-overlap/train.jsonl")
	eval := testutil.Fixture(t, "cross-dataset-overlap/eval.jsonl")
	test := testutil.Fixture(t, "cross-dataset-overlap/test.jsonl")

	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Datasets: map[string][]string{
			"train": {train},
			"eval":  {eval},
			"test":  {test},
		},
	}, crossDatasetOverlapRuleID)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}

	for i, f := range got {
		if !strings.HasSuffix(f.Location.Path, "train.jsonl") {
			t.Errorf("finding %d path = %q, want suffix train.jsonl", i, f.Location.Path)
		}
	}
	wantRows := []int{2, 1} // (eval, train) hits row 2; (test, train) hits row 1
	for i, f := range got {
		if f.Location.Row != wantRows[i] {
			t.Errorf("finding %d row = %d, want %d", i, f.Location.Row, wantRows[i])
		}
	}
	if !strings.Contains(got[0].Message, `"eval"`) {
		t.Errorf("first message should cite eval as the source dataset: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, `"train"`) {
		t.Errorf("first message should cite train as the host dataset: %q", got[0].Message)
	}
	if !strings.Contains(got[1].Message, `"test"`) {
		t.Errorf("second message should cite test as the source dataset: %q", got[1].Message)
	}
}

func TestCrossDatasetOverlap_TwoCleanDatasetsNoFire(t *testing.T) {
	// eval and test share nothing — no findings.
	eval := testutil.Fixture(t, "cross-dataset-overlap/eval.jsonl")
	test := testutil.Fixture(t, "cross-dataset-overlap/test.jsonl")

	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Datasets: map[string][]string{
			"eval": {eval},
			"test": {test},
		},
	}, crossDatasetOverlapRuleID)

	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestCrossDatasetOverlap_SingleDatasetIsNoOp(t *testing.T) {
	// One dataset can't overlap with itself; rule should be silent.
	train := testutil.Fixture(t, "cross-dataset-overlap/train.jsonl")
	got := corpusFindingsForRule(t, &rules.CorpusContext{
		Datasets: map[string][]string{"train": {train}},
	}, crossDatasetOverlapRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on a single dataset, got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestCrossDatasetOverlap_NearDupConfigOverride(t *testing.T) {
	// Use the near-dup fixture pair from train-eval-overlap as two
	// named datasets. With near_dup_threshold=0.4 (well below the
	// MinHash test's calibration band) the pair should produce one
	// near-duplicate finding citing similarity in the message.
	train := testutil.Fixture(t, "train-eval-overlap/train-near-dup.jsonl")
	eval := testutil.Fixture(t, "train-eval-overlap/eval-near-dup.jsonl")

	cfg := config.Default()
	cfg.Rules[crossDatasetOverlapRuleID] = map[string]any{
		"near_dup_threshold": 0.4,
	}

	all := pipeline.RunCorpus(&rules.CorpusContext{
		Datasets: map[string][]string{
			"train": {train},
			"eval":  {eval},
		},
	}, cfg)
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == crossDatasetOverlapRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 near-dup finding, got %d: %s", len(got), joinMessages(got))
	}
	if !strings.Contains(got[0].Message, "near-duplicate") {
		t.Errorf("message should call out near-duplicate: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "similarity") {
		t.Errorf("message should report similarity: %q", got[0].Message)
	}
}
