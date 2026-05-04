package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/testutil"
)

const dedupNormalizationRuleID = "dedup-key-misses-normalization"

func TestDedupNormalization_DropDuplicates(t *testing.T) {
	path := testutil.Fixture(t, "dedup-key-misses-normalization/positive.py")
	got := findingsForRule(t, path, dedupNormalizationRuleID)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Line != 4 {
		t.Errorf("line = %d, want 4", got[0].Location.Line)
	}
	if !strings.Contains(got[0].Message, "drop_duplicates") {
		t.Errorf("message missing call name: %q", got[0].Message)
	}
}

func TestDedupNormalization_BareSet(t *testing.T) {
	// Builtin set() used as dedup. Confidence is intentionally low —
	// set([1, 2, 3]) is also dedup but doesn't need text norm — so
	// users will mute this via Config.Disable when noisy.
	path := testutil.Fixture(t, "dedup-key-misses-normalization/positive-set.py")
	got := findingsForRule(t, path, dedupNormalizationRuleID)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if !strings.Contains(got[0].Message, "set called") {
		t.Errorf("message should name set: %q", got[0].Message)
	}
}

func TestDedupNormalization_LowerPresent(t *testing.T) {
	// .str.lower() and .str.strip() before drop_duplicates → rule
	// must stay silent.
	path := testutil.Fixture(t, "dedup-key-misses-normalization/negative-with-lower.py")
	got := findingsForRule(t, path, dedupNormalizationRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (lower present), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestDedupNormalization_UnicodeDataPresent(t *testing.T) {
	// unicodedata.normalize() → calleeBaseName returns "normalize",
	// rule stays silent.
	path := testutil.Fixture(t, "dedup-key-misses-normalization/negative-with-unicodedata.py")
	got := findingsForRule(t, path, dedupNormalizationRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (unicodedata.normalize present), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestDedupNormalization_NoDedup(t *testing.T) {
	path := testutil.Fixture(t, "dedup-key-misses-normalization/negative-no-dedup.py")
	got := findingsForRule(t, path, dedupNormalizationRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (no dedup), got %d: %s",
			len(got), joinMessages(got))
	}
}
