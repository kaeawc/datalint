package main

import (
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
)

func TestParseSeverity(t *testing.T) {
	cases := []struct {
		in      string
		want    diag.Severity
		wantErr bool
	}{
		{"info", diag.SeverityInfo, false},
		{"warning", diag.SeverityWarning, false},
		{"error", diag.SeverityError, false},
		{"none", 0, true}, // none is handled by caller, not parseSeverity
		{"INFO", 0, true},
		{"", 0, true},
		{"critical", 0, true},
	}
	for _, c := range cases {
		got, err := parseSeverity(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSeverity(%q) = (%v, nil), want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSeverity(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSeverity(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	mk := func(s diag.Severity, id string) diag.Finding {
		return diag.Finding{Severity: s, RuleID: id}
	}
	all := []diag.Finding{
		mk(diag.SeverityInfo, "i"),
		mk(diag.SeverityWarning, "w"),
		mk(diag.SeverityError, "e"),
	}

	cases := []struct {
		level   string
		wantIDs []string
		wantErr bool
	}{
		{"none", []string{"i", "w", "e"}, false},
		{"info", []string{"i", "w", "e"}, false},
		{"warning", []string{"w", "e"}, false},
		{"error", []string{"e"}, false},
		{"critical", nil, true},
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			got, err := filterBySeverity(all, c.level)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(c.wantIDs) {
				t.Fatalf("len = %d, want %d (got %+v)", len(got), len(c.wantIDs), got)
			}
			for i, id := range c.wantIDs {
				if got[i].RuleID != id {
					t.Errorf("idx %d: got %q, want %q", i, got[i].RuleID, id)
				}
			}
		})
	}
}

func TestFilterBySeverity_PreservesUnderlyingForFailOn(t *testing.T) {
	// Regression: --min-severity must not affect --fail-on.
	mk := func(s diag.Severity) diag.Finding { return diag.Finding{Severity: s} }
	findings := []diag.Finding{mk(diag.SeverityInfo), mk(diag.SeverityError)}

	displayed, err := filterBySeverity(findings, "error")
	if err != nil {
		t.Fatal(err)
	}
	if len(displayed) != 1 {
		t.Fatalf("expected 1 displayed finding, got %d", len(displayed))
	}
	// The original slice must still hold both entries — main.go
	// passes the unfiltered slice to exitCodeForFailOn after this
	// filter call.
	if len(findings) != 2 {
		t.Fatalf("filterBySeverity mutated input: len(findings) = %d, want 2", len(findings))
	}
}

func TestExitCodeForFailOn(t *testing.T) {
	mk := func(s diag.Severity) diag.Finding { return diag.Finding{Severity: s} }
	infos := []diag.Finding{mk(diag.SeverityInfo)}
	warns := []diag.Finding{mk(diag.SeverityInfo), mk(diag.SeverityWarning)}
	errs := []diag.Finding{mk(diag.SeverityWarning), mk(diag.SeverityError)}

	cases := []struct {
		name     string
		level    string
		findings []diag.Finding
		want     int
		wantErr  bool
	}{
		{"none always 0", "none", errs, 0, false},
		{"info catches info", "info", infos, 1, false},
		{"info catches warning", "info", warns, 1, false},
		{"info catches error", "info", errs, 1, false},
		{"warning skips info", "warning", infos, 0, false},
		{"warning catches warning", "warning", warns, 1, false},
		{"warning catches error", "warning", errs, 1, false},
		{"error skips info", "error", infos, 0, false},
		{"error skips warning-only", "error", warns[:1], 0, false},
		{"error catches error", "error", errs, 1, false},
		{"empty findings exit 0", "error", nil, 0, false},
		{"bad level returns error", "critical", errs, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := exitCodeForFailOn(c.level, c.findings)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for level=%q", c.level)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("exit = %d, want %d", got, c.want)
			}
		})
	}
}
