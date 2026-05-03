package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaeawc/datalint/internal/scanner"
)

func TestParsePython_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.py")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	pf, err := scanner.ParsePython(path)
	if err != nil {
		t.Fatalf("ParsePython: %v", err)
	}
	if pf.Tree == nil {
		t.Fatal("Tree is nil")
	}
	if pf.Tree.RootNode() == nil {
		t.Fatal("RootNode is nil")
	}
	if pf.Tree.RootNode().Type() != "module" {
		t.Errorf("root type = %q, want module", pf.Tree.RootNode().Type())
	}
}

func TestParsePython_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.py")
	source := "x = 1\ny = x + 2\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	pf, err := scanner.ParsePython(path)
	if err != nil {
		t.Fatalf("ParsePython: %v", err)
	}
	if string(pf.Source) != source {
		t.Errorf("Source = %q, want %q", string(pf.Source), source)
	}
	root := pf.Tree.RootNode()
	if root.ChildCount() == 0 {
		t.Fatal("expected at least one child")
	}
}

func TestParsePython_FileNotFound(t *testing.T) {
	_, err := scanner.ParsePython("/nonexistent-path-for-test.py")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
