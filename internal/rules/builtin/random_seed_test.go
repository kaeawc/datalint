package builtin_test

import (
	"strings"
	"testing"

	_ "github.com/kaeawc/datalint/internal/rules/builtin"
	"github.com/kaeawc/datalint/internal/testutil"
)

const randomSeedRuleID = "random-seed-not-set"

func TestRandomSeedNotSet_PythonStdlib(t *testing.T) {
	path := testutil.Fixture(t, "random-seed-not-set/positive.py")
	got := findingsForRule(t, path, randomSeedRuleID)

	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Line != 4 {
		t.Errorf("line = %d, want 4", got[0].Location.Line)
	}
	if !strings.Contains(got[0].Message, "random.shuffle") {
		t.Errorf("message missing call name: %q", got[0].Message)
	}
}

func TestRandomSeedNotSet_NumpyMultiple(t *testing.T) {
	path := testutil.Fixture(t, "random-seed-not-set/positive-numpy.py")
	got := findingsForRule(t, path, randomSeedRuleID)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %s", len(got), joinMessages(got))
	}
	calls := make([]string, 0, len(got))
	for _, f := range got {
		calls = append(calls, f.Message)
	}
	joined := strings.Join(calls, " | ")
	if !strings.Contains(joined, "np.random.permutation") {
		t.Errorf("missing np.random.permutation finding: %s", joined)
	}
	if !strings.Contains(joined, "np.random.choice") {
		t.Errorf("missing np.random.choice finding: %s", joined)
	}
}

func TestRandomSeedNotSet_Seeded(t *testing.T) {
	path := testutil.Fixture(t, "random-seed-not-set/negative-seeded.py")
	got := findingsForRule(t, path, randomSeedRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (random.seed clears the file), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestRandomSeedNotSet_NoRNGCalls(t *testing.T) {
	path := testutil.Fixture(t, "random-seed-not-set/negative-no-rng.py")
	got := findingsForRule(t, path, randomSeedRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestRandomSeedNotSet_NonPythonPathSkipped(t *testing.T) {
	path := testutil.Fixture(t, "jsonl-malformed-line/negative.jsonl")
	got := findingsForRule(t, path, randomSeedRuleID)
	if len(got) != 0 {
		t.Errorf("rule should not fire on a JSONL file: %d findings", len(got))
	}
}
