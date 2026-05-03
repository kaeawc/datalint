package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaeawc/datalint/internal/scanner"
)

func TestStreamJSONL_RowIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.jsonl")
	contents := "a\nb\n\nc"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	var rows []int
	var lines []string
	err := scanner.StreamJSONL(path, func(row int, line []byte) error {
		rows = append(rows, row)
		lines = append(lines, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("StreamJSONL: %v", err)
	}

	wantRows := []int{1, 2, 3, 4}
	wantLines := []string{"a", "b", "", "c"}
	if !equalInts(rows, wantRows) {
		t.Errorf("rows = %v, want %v", rows, wantRows)
	}
	if !equalStrings(lines, wantLines) {
		t.Errorf("lines = %v, want %v", lines, wantLines)
	}
}

func TestStreamJSONL_CRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.jsonl")
	if err := os.WriteFile(path, []byte("a\r\nb\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var lines []string
	err := scanner.StreamJSONL(path, func(_ int, line []byte) error {
		lines = append(lines, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("StreamJSONL: %v", err)
	}
	want := []string{"a", "b"}
	if !equalStrings(lines, want) {
		t.Errorf("lines = %v, want %v", lines, want)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		path string
		want scanner.Kind
	}{
		{"foo.jsonl", scanner.KindJSONL},
		{"foo.py", scanner.KindPythonSource},
		{"foo.parquet", scanner.KindParquet},
		{"foo.txt", scanner.KindUnknown},
	}
	for _, tc := range cases {
		got := scanner.Classify(tc.path)
		if got.Kind != tc.want {
			t.Errorf("Classify(%q).Kind = %v, want %v", tc.path, got.Kind, tc.want)
		}
		if got.Path != tc.path {
			t.Errorf("Classify(%q).Path = %q", tc.path, got.Path)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
