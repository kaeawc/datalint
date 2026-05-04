package fixer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/fixer"
)

func TestApply_InsertsBeforeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	original := "import random\n\nrandom.shuffle(data)\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	findings := []diag.Finding{
		{
			RuleID:   "x",
			Location: diag.Location{Path: path, Line: 3},
			Fix: &diag.Fix{
				Description: "seed",
				Level:       diag.FixIdiomatic,
				Edits:       []diag.FixEdit{{Line: 2, Insert: "random.seed(0)\n"}},
			},
		},
	}
	res, err := fixer.Apply(findings)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.FilesModified != 1 || res.EditsApplied != 1 {
		t.Errorf("Result = %+v, want FilesModified=1 EditsApplied=1", res)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "import random\nrandom.seed(0)\n\nrandom.shuffle(data)\n"
	if string(got) != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestApply_DedupsIdenticalEdits(t *testing.T) {
	// Two findings, same path, same edit (a rule fires on every
	// unseeded RNG call but emits the same insert). Apply should
	// insert once.
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	if err := os.WriteFile(path, []byte("import random\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	edit := diag.FixEdit{Line: 2, Insert: "random.seed(0)\n"}
	findings := []diag.Finding{
		{Location: diag.Location{Path: path, Line: 1}, Fix: &diag.Fix{Edits: []diag.FixEdit{edit}}},
		{Location: diag.Location{Path: path, Line: 5}, Fix: &diag.Fix{Edits: []diag.FixEdit{edit}}},
		{Location: diag.Location{Path: path, Line: 9}, Fix: &diag.Fix{Edits: []diag.FixEdit{edit}}},
	}
	res, err := fixer.Apply(findings)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.EditsApplied != 1 {
		t.Errorf("EditsApplied = %d, want 1 (dedup)", res.EditsApplied)
	}
	got, _ := os.ReadFile(path)
	want := "import random\nrandom.seed(0)\n"
	if string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestApply_MultipleEditsReverseOrder(t *testing.T) {
	// Two distinct edits at different lines. Reverse-line ordering
	// means the line=5 insert happens first (no shift), then the
	// line=2 insert happens at the still-correct line=2.
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	original := "a\nb\nc\nd\ne\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	findings := []diag.Finding{
		{Location: diag.Location{Path: path}, Fix: &diag.Fix{
			Edits: []diag.FixEdit{
				{Line: 2, Insert: "X\n"},
				{Line: 5, Insert: "Y\n"},
			},
		}},
	}
	if _, err := fixer.Apply(findings); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "a\nX\nb\nc\nd\nY\ne\n"
	if string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestApply_NoFixIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	original := "noop\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	findings := []diag.Finding{
		{Location: diag.Location{Path: path}}, // Fix is nil
	}
	res, err := fixer.Apply(findings)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesModified != 0 || res.EditsApplied != 0 {
		t.Errorf("Result = %+v, want zeros", res)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Errorf("file modified despite no Fix; got %q", got)
	}
}
