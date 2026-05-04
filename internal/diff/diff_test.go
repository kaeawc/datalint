package diff_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/diff"
)

func writeJSONL(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCompute_RowCountAndFieldDelta(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// old: 3 rows, fields {id, name, score}
	writeJSONL(t, oldPath,
		"{\"id\": 1, \"name\": \"a\", \"score\": 0.1}\n"+
			"{\"id\": 2, \"name\": \"b\", \"score\": 0.2}\n"+
			"{\"id\": 3, \"name\": \"c\", \"score\": 0.3}\n")

	// new: 5 rows, fields {id, name, label} — score removed, label added
	writeJSONL(t, newPath,
		"{\"id\": 1, \"name\": \"a\", \"label\": \"x\"}\n"+
			"{\"id\": 2, \"name\": \"b\", \"label\": \"y\"}\n"+
			"{\"id\": 3, \"name\": \"c\", \"label\": \"z\"}\n"+
			"{\"id\": 4, \"name\": \"d\", \"label\": \"x\"}\n"+
			"{\"id\": 5, \"name\": \"e\", \"label\": \"y\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if r.OldRows != 3 {
		t.Errorf("OldRows = %d, want 3", r.OldRows)
	}
	if r.NewRows != 5 {
		t.Errorf("NewRows = %d, want 5", r.NewRows)
	}
	if !equalStrings(r.Added, []string{"label"}) {
		t.Errorf("Added = %v, want [label]", r.Added)
	}
	if !equalStrings(r.Removed, []string{"score"}) {
		t.Errorf("Removed = %v, want [score]", r.Removed)
	}
	if !equalStrings(r.Common, []string{"id", "name"}) {
		t.Errorf("Common = %v, want [id, name]", r.Common)
	}
}

func TestCompute_MalformedRowsSkipped(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	writeJSONL(t, oldPath, "{\"id\": 1}\nnot json\n{\"id\": 2}\n")
	writeJSONL(t, newPath, "{\"id\": 1}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	// Old has 2 parseable rows (the malformed one in the middle is
	// skipped), new has 1.
	if r.OldRows != 2 {
		t.Errorf("OldRows = %d, want 2 (malformed line skipped)", r.OldRows)
	}
	if r.NewRows != 1 {
		t.Errorf("NewRows = %d, want 1", r.NewRows)
	}
}

func TestCompute_EmptyFiles(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	writeJSONL(t, oldPath, "")
	writeJSONL(t, newPath, "")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if r.OldRows != 0 || r.NewRows != 0 {
		t.Errorf("rows = (%d, %d), want (0, 0)", r.OldRows, r.NewRows)
	}
	if len(r.Added) != 0 || len(r.Removed) != 0 || len(r.Common) != 0 {
		t.Errorf("expected empty field sets; added=%v removed=%v common=%v",
			r.Added, r.Removed, r.Common)
	}
}

func TestCompute_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.jsonl")
	writeJSONL(t, good, "{}\n")
	if _, err := diff.Compute("/nonexistent.jsonl", good); err == nil {
		t.Error("expected error when old path missing")
	}
	if _, err := diff.Compute(good, "/nonexistent.jsonl"); err == nil {
		t.Error("expected error when new path missing")
	}
}

func TestWriteText_DeltaSign(t *testing.T) {
	cases := []struct {
		name      string
		report    diff.Report
		wantInOut []string
		notInOut  []string
	}{
		{
			name:   "growth uses + sign",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 5, NewRows: 8, Added: []string{"x"}},
			wantInOut: []string{
				"old: a  (5 rows)",
				"new: b  (8 rows; +3)",
				"fields added:    [x]",
			},
		},
		{
			name:   "shrinkage uses - sign (no extra +)",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 10, NewRows: 4, Removed: []string{"y"}},
			wantInOut: []string{
				"new: b  (4 rows; -6)",
				"fields removed:  [y]",
			},
			notInOut: []string{"+-6"},
		},
		{
			name:   "zero delta",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 7, NewRows: 7},
			wantInOut: []string{
				"new: b  (7 rows; 0)",
			},
			notInOut: []string{"+0", "-0"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := diff.WriteText(&buf, c.report); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, want := range c.wantInOut {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in:\n%s", want, out)
				}
			}
			for _, unwanted := range c.notInOut {
				if strings.Contains(out, unwanted) {
					t.Errorf("unexpected %q in:\n%s", unwanted, out)
				}
			}
		})
	}
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
