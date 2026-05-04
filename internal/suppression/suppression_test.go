package suppression_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/suppression"
)

func TestExtractFromPython(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	body := `import random
random.shuffle(data)  # datalint:disable=random-seed-not-set
random.shuffle(data)  # comment with no marker
random.shuffle(data)  # datalint:disable=random-seed-not-set, other-rule
# datalint:disable=role-inversion
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := suppression.ExtractFromPython(path)

	if !s.Suppresses(mkPyFinding(path, "random-seed-not-set", 2)) {
		t.Error("line 2 should suppress random-seed-not-set")
	}
	if s.Suppresses(mkPyFinding(path, "random-seed-not-set", 3)) {
		t.Error("line 3 has no marker; should not suppress")
	}
	if !s.Suppresses(mkPyFinding(path, "other-rule", 4)) {
		t.Error("line 4 should suppress other-rule (multi-id form)")
	}
	if !s.Suppresses(mkPyFinding(path, "role-inversion", 5)) {
		t.Error("line 5 (full-line comment) should suppress")
	}
	if s.Suppresses(mkPyFinding(path, "different-rule", 2)) {
		t.Error("line 2 disables random-seed-not-set, not different-rule")
	}
}

func TestExtractFromJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.jsonl")
	body := `{"label": "x"}
{"label": "y", "_datalint_disable": ["enum-drift"]}
{"label": "z", "_datalint_disable": ["enum-drift", "role-inversion"]}
not even json
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := suppression.ExtractFromJSONL(path)

	if s.Suppresses(mkJSONLFinding(path, "enum-drift", 1)) {
		t.Error("row 1 has no disable")
	}
	if !s.Suppresses(mkJSONLFinding(path, "enum-drift", 2)) {
		t.Error("row 2 should suppress enum-drift")
	}
	if !s.Suppresses(mkJSONLFinding(path, "role-inversion", 3)) {
		t.Error("row 3 should suppress role-inversion")
	}
	if s.Suppresses(mkJSONLFinding(path, "role-inversion", 2)) {
		t.Error("row 2 only disables enum-drift")
	}
	// Row 4 is malformed; suppression set has no entry for it.
	if s.Suppresses(mkJSONLFinding(path, "any", 4)) {
		t.Error("malformed row can't carry a disable")
	}
}

func TestExtractFromFile_UnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := suppression.ExtractFromFile(path)
	if s.Suppresses(mkPyFinding(path, "any", 1)) {
		t.Error("unknown extensions should yield empty suppression")
	}
}

func TestFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	if err := os.WriteFile(path, []byte("x = 1  # datalint:disable=keep-me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	findings := []diag.Finding{
		{RuleID: "keep-me", Location: diag.Location{Path: path, Line: 1}},
		{RuleID: "drop-me", Location: diag.Location{Path: path, Line: 1}},
		{RuleID: "keep-me", Location: diag.Location{Path: path, Line: 2}},
	}
	got := suppression.Filter(findings)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings to survive, got %d", len(got))
	}
	if got[0].RuleID != "drop-me" || got[1].RuleID != "keep-me" {
		t.Errorf("survivors out of order or wrong: %+v", got)
	}
}

func mkPyFinding(path, rule string, line int) diag.Finding {
	return diag.Finding{
		RuleID:   rule,
		Location: diag.Location{Path: path, Line: line},
	}
}

func mkJSONLFinding(path, rule string, row int) diag.Finding {
	return diag.Finding{
		RuleID:   rule,
		Location: diag.Location{Path: path, Row: row},
	}
}
