package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const shuffleAfterSplitRuleID = "shuffle-after-split"

func TestShuffleAfterSplit_Stdlib(t *testing.T) {
	path := testutil.Fixture(t, "shuffle-after-split/positive.py")
	got := findingsForRule(t, path, shuffleAfterSplitRuleID)

	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Line != 7 {
		t.Errorf("line = %d, want 7", got[0].Location.Line)
	}
	if !strings.Contains(got[0].Message, "random.shuffle") {
		t.Errorf("message missing call name: %q", got[0].Message)
	}
}

func TestShuffleAfterSplit_Numpy(t *testing.T) {
	path := testutil.Fixture(t, "shuffle-after-split/positive-numpy.py")
	got := findingsForRule(t, path, shuffleAfterSplitRuleID)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if !strings.Contains(got[0].Message, "np.random.permutation") {
		t.Errorf("message missing call name: %q", got[0].Message)
	}
}

func TestShuffleAfterSplit_ShuffleFirst(t *testing.T) {
	path := testutil.Fixture(t, "shuffle-after-split/negative-shuffle-first.py")
	got := findingsForRule(t, path, shuffleAfterSplitRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (shuffle precedes split), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestShuffleAfterSplit_NestedCallNotFlagged(t *testing.T) {
	// `train_test_split(random.shuffle(data) or data)` — the inner
	// shuffle executes first despite its source position. The
	// isNestedInCall guard must drop both the inner shuffle and the
	// inner split (only the outer counts).
	path := testutil.Fixture(t, "shuffle-after-split/negative-nested-call.py")
	got := findingsForRule(t, path, shuffleAfterSplitRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (nested call), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestShuffleAfterSplit_NoSplitInFile(t *testing.T) {
	path := testutil.Fixture(t, "shuffle-after-split/negative-no-split.py")
	got := findingsForRule(t, path, shuffleAfterSplitRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (no split call), got %d: %s",
			len(got), joinMessages(got))
	}
}
