package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/output"
)

func TestWriteDrops_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, nil); err != nil {
		t.Fatalf("WriteDrops: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestWriteDrops_OneRowOneRule(t *testing.T) {
	findings := []diag.Finding{
		{
			RuleID:   "jsonl-malformed-line",
			Location: diag.Location{Path: "data.jsonl", Row: 5},
		},
	}
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, findings); err != nil {
		t.Fatal(err)
	}
	want := "data.jsonl\t5\tjsonl-malformed-line\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestWriteDrops_AggregatesRulesPerRow(t *testing.T) {
	// Same (path, row) hit by two rules → one line, comma-joined
	// alphabetical rule list.
	findings := []diag.Finding{
		{RuleID: "role-inversion", Location: diag.Location{Path: "data.jsonl", Row: 3}},
		{RuleID: "unbalanced-tool-call-id", Location: diag.Location{Path: "data.jsonl", Row: 3}},
	}
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, findings); err != nil {
		t.Fatal(err)
	}
	want := "data.jsonl\t3\trole-inversion,unbalanced-tool-call-id\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestWriteDrops_SortedByPathAndRow(t *testing.T) {
	findings := []diag.Finding{
		{RuleID: "x", Location: diag.Location{Path: "b.jsonl", Row: 2}},
		{RuleID: "x", Location: diag.Location{Path: "a.jsonl", Row: 5}},
		{RuleID: "x", Location: diag.Location{Path: "a.jsonl", Row: 2}},
	}
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, findings); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	want := []string{
		"a.jsonl\t2\tx",
		"a.jsonl\t5\tx",
		"b.jsonl\t2\tx",
	}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWriteDrops_SkipsLineOnlyFindings(t *testing.T) {
	// Python AST findings carry Line, not Row. They aren't actionable
	// as data-row drops, so the format excludes them entirely.
	findings := []diag.Finding{
		{RuleID: "random-seed-not-set", Location: diag.Location{Path: "pipeline.py", Line: 4}},
		{RuleID: "jsonl-malformed-line", Location: diag.Location{Path: "data.jsonl", Row: 7}},
	}
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, findings); err != nil {
		t.Fatal(err)
	}
	want := "data.jsonl\t7\tjsonl-malformed-line\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q (Python finding should be excluded)", buf.String(), want)
	}
}

func TestWriteDrops_SkipsEmptyPath(t *testing.T) {
	findings := []diag.Finding{
		{RuleID: "x", Location: diag.Location{Row: 5}}, // Path empty
	}
	var buf bytes.Buffer
	if err := output.WriteDrops(&buf, findings); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for path-less finding, got %q", buf.String())
	}
}
