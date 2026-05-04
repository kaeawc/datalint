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
