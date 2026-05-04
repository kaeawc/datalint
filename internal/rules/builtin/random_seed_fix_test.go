package builtin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/fixer"
	"github.com/kaeawc/datalint/internal/pipeline"
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

// TestRandomSeedNotSet_FixIsIdempotent walks the full --fix loop:
// run datalint, get findings with a Fix attached, apply, re-run,
// confirm no findings. Catches regressions where the fix doesn't
// actually clear the rule's signal.
func TestRandomSeedNotSet_FixIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	original := "import random\nimport os\n\nrandom.shuffle(data)\nrandom.shuffle(data)\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	rng := filterByRule(first, "random-seed-not-set")
	if len(rng) != 2 {
		t.Fatalf("expected 2 random-seed-not-set findings before fix, got %d", len(rng))
	}
	for _, f := range rng {
		if f.Fix == nil {
			t.Errorf("finding %+v missing Fix", f)
			continue
		}
		if f.Fix.Level != diag.FixIdiomatic {
			t.Errorf("Fix.Level = %q, want idiomatic", f.Fix.Level)
		}
	}

	res, err := fixer.Apply(first)
	if err != nil {
		t.Fatalf("fixer.Apply: %v", err)
	}
	if res.FilesModified != 1 {
		t.Errorf("FilesModified = %d, want 1", res.FilesModified)
	}
	if res.EditsApplied != 1 {
		// Two findings produced the same edit — dedup should land it
		// at one applied edit, not two.
		t.Errorf("EditsApplied = %d, want 1 (dedup)", res.EditsApplied)
	}

	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "random.seed(0)") {
		t.Errorf("file lacks random.seed(0) after fix:\n%s", got)
	}
	// Verify the seed call sits between the imports and the shuffle.
	importIdx := strings.Index(string(got), "import os")
	seedIdx := strings.Index(string(got), "random.seed(0)")
	shuffleIdx := strings.Index(string(got), "random.shuffle")
	if importIdx >= seedIdx || seedIdx >= shuffleIdx {
		t.Errorf("seed insertion is in the wrong place:\n%s", got)
	}

	second, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if rng2 := filterByRule(second, "random-seed-not-set"); len(rng2) != 0 {
		t.Errorf("rule still fires after fix: %d findings", len(rng2))
	}
}

func TestRandomSeedNotSet_FixUsesNumpyVariant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	original := "import numpy as np\n\nnp.random.shuffle(data)\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := pipeline.Run([]string{path}, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	rng := filterByRule(findings, "random-seed-not-set")
	if len(rng) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(rng))
	}
	if rng[0].Fix == nil {
		t.Fatal("Fix is nil")
	}
	if !strings.Contains(rng[0].Fix.Edits[0].Insert, "np.random.seed") {
		t.Errorf("fix should use np.random.seed, got %q", rng[0].Fix.Edits[0].Insert)
	}
}

func filterByRule(findings []diag.Finding, id string) []diag.Finding {
	var out []diag.Finding
	for _, f := range findings {
		if f.RuleID == id {
			out = append(out, f)
		}
	}
	return out
}
