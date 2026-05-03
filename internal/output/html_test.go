package output_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/output"
)

func renderHTML(t *testing.T, findings []diag.Finding) string {
	t.Helper()
	var buf bytes.Buffer
	pinned := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	if err := output.WriteHTML(&buf, findings, "1.2.3", pinned); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	return buf.String()
}

func TestWriteHTML_Empty(t *testing.T) {
	got := renderHTML(t, nil)
	mustContain(t, got, []string{
		"<!doctype html>",
		"datalint report",
		"version 1.2.3",
		"generated 2026-05-03T12:00:00Z",
		"0 findings across 0 rules",
		"No findings.",
	})
	mustNotContain(t, got, []string{
		"<table>", // summary table is suppressed when there are no findings
		"<section class=\"rule\"",
	})
}

func TestWriteHTML_GroupsByRule(t *testing.T) {
	findings := []diag.Finding{
		{
			RuleID:   "jsonl-malformed-line",
			Severity: diag.SeverityError,
			Message:  "blank line in JSONL",
			Location: diag.Location{Path: "data.jsonl", Row: 4},
		},
		{
			RuleID:   "jsonl-malformed-line",
			Severity: diag.SeverityError,
			Message:  "invalid JSON",
			Location: diag.Location{Path: "data.jsonl", Row: 2},
		},
		{
			RuleID:   "field-type-mixed-across-rows",
			Severity: diag.SeverityWarning,
			Message:  `field "score" has mixed types`,
			Location: diag.Location{Path: "data.jsonl", Row: 3},
		},
	}
	got := renderHTML(t, findings)

	mustContain(t, got, []string{
		"3 findings across 2 rules",
		`id="jsonl-malformed-line"`,
		`id="field-type-mixed-across-rows"`,
		"sev-error",
		"sev-warning",
		"data.jsonl:2",
		"data.jsonl:3",
		"data.jsonl:4",
	})

	// Summary table should appear.
	if !strings.Contains(got, "<table>") {
		t.Error("summary <table> missing when findings exist")
	}

	// Sections appear in alphabetical rule order.
	idxField := strings.Index(got, `id="field-type-mixed-across-rows"`)
	idxJSONL := strings.Index(got, `id="jsonl-malformed-line"`)
	if idxField <= 0 || idxJSONL <= 0 || idxField > idxJSONL {
		t.Errorf("rule sections out of order: field@%d jsonl@%d", idxField, idxJSONL)
	}

	// HTML escaping: the literal quote in the field-type message must
	// be encoded so the page renders correctly.
	if !strings.Contains(got, "&#34;score&#34;") && !strings.Contains(got, "&quot;score&quot;") {
		t.Error("expected message to be HTML-escaped")
	}
}

func TestWriteHTML_LocationFallbacks(t *testing.T) {
	cases := []struct {
		name string
		loc  diag.Location
		want string
	}{
		{"path with line", diag.Location{Path: "f.py", Line: 12}, "f.py:12"},
		{"path with row only", diag.Location{Path: "f.jsonl", Row: 7}, "f.jsonl:7"},
		{"path no line/row", diag.Location{Path: "f.py"}, "f.py</code>"},
		{"empty path", diag.Location{}, "(no location)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := []diag.Finding{{
				RuleID:   "x",
				Severity: diag.SeverityInfo,
				Message:  "m",
				Location: tc.loc,
			}}
			got := renderHTML(t, findings)
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected %q in output", tc.want)
			}
		})
	}
}

func mustContain(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("expected %q in output", w)
		}
	}
}

func mustNotContain(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, w := range wants {
		if strings.Contains(got, w) {
			t.Errorf("unexpected %q in output", w)
		}
	}
}
