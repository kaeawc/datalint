// Package smoke exercises the full rule registry end-to-end against
// a curated, intentionally-buggy fixture corpus. The expected counts
// catch regressions where a rule silently stops firing or starts
// over-firing — they're a fast complement to the per-rule unit tests.
//
// When a rule's output legitimately changes (new positive case, new
// false-positive guard), update the expected counts here in the same
// PR.
package smoke_test

import (
	"sort"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/rules"
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
	"github.com/kaeawc/datalint/internal/testutil"
)

func TestSmoke_PerFileCorpus(t *testing.T) {
	paths := []string{
		testutil.Fixture(t, "smoke-corpus/conversations.jsonl"),
		testutil.Fixture(t, "smoke-corpus/labels.jsonl"),
		testutil.Fixture(t, "smoke-corpus/pipeline.py"),
	}
	findings, err := pipeline.Run(paths, config.Default())
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}

	// Expected counts come from manual inspection of the fixtures.
	// See tests/fixtures/smoke-corpus/* for the buggy patterns.
	expected := map[string]int{
		"jsonl-malformed-line":                  1,
		"role-inversion":                        1,
		"system-prompt-leaks-eval-instructions": 1,
		"tool-result-without-tool-call":         2,
		"unbalanced-tool-call-id":               1,
		"enum-drift":                            1,
		"field-type-mixed-across-rows":          1,
		"random-seed-not-set":                   2,
		"shuffle-after-split":                   1,
		"dedup-key-misses-normalization":        1,
	}
	assertCounts(t, findings, expected)
}

func TestSmoke_CorpusScope(t *testing.T) {
	corpus := &rules.CorpusContext{
		Train: []string{testutil.Fixture(t, "smoke-corpus/train.jsonl")},
		Eval:  []string{testutil.Fixture(t, "smoke-corpus/eval.jsonl")},
	}
	findings := pipeline.RunCorpus(corpus, config.Default())

	// Eval row 1 ("Define photosynthesis.") and row 3 ("What is
	// the capital of France?") both appear verbatim in train.
	expected := map[string]int{
		"train-eval-overlap": 2,
	}
	assertCounts(t, findings, expected)
}

func assertCounts(t *testing.T, findings []diag.Finding, expected map[string]int) {
	t.Helper()
	got := map[string]int{}
	for _, f := range findings {
		got[f.RuleID]++
	}

	// Walk both maps so we report under-firing AND over-firing.
	keys := unionKeys(got, expected)
	for _, id := range keys {
		if got[id] != expected[id] {
			t.Errorf("rule %s: got %d findings, want %d", id, got[id], expected[id])
		}
	}
}

func unionKeys(a, b map[string]int) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
