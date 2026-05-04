// Package diff compares two JSONL dataset versions and reports
// distribution shifts: row count, field set deltas, and (in the v0
// scope) presence ratios per field.
//
// The output deliberately isn't a Finding — diff mode answers a
// different question ("what changed between dataset v1 and v2?")
// than the rule pipeline ("is this dataset internally consistent?").
package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kaeawc/datalint/internal/scanner"
)

// Report is the diff between an "old" and "new" JSONL file. Counts
// are total parseable rows (malformed rows are not counted).
type Report struct {
	OldPath string
	NewPath string
	OldRows int
	NewRows int
	Added   []string // top-level field names present in new but not old
	Removed []string // top-level field names present in old but not new
	Common  []string // top-level field names present in both
}

// Compute streams both files once each, recording the set of
// top-level field names seen and the parseable-row count. Malformed
// rows are silently skipped — a separate jsonl-malformed-line lint
// run is the right place to surface them.
func Compute(oldPath, newPath string) (Report, error) {
	oldRows, oldFields, err := scanFields(oldPath)
	if err != nil {
		return Report{}, fmt.Errorf("scan old: %w", err)
	}
	newRows, newFields, err := scanFields(newPath)
	if err != nil {
		return Report{}, fmt.Errorf("scan new: %w", err)
	}
	return Report{
		OldPath: oldPath,
		NewPath: newPath,
		OldRows: oldRows,
		NewRows: newRows,
		Added:   sortedDiff(newFields, oldFields),
		Removed: sortedDiff(oldFields, newFields),
		Common:  sortedIntersection(oldFields, newFields),
	}, nil
}

func scanFields(path string) (int, map[string]bool, error) {
	rows := 0
	fields := map[string]bool{}
	err := scanner.StreamJSONL(path, func(_ int, line []byte) error {
		recordRow(&rows, fields, line)
		return nil
	})
	return rows, fields, err
}

// recordRow parses one JSONL row and updates the running stats.
// Extracted so the StreamJSONL closure has no error-returning
// surface (nilerr pattern from PR #1 / #7).
func recordRow(rows *int, fields map[string]bool, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	*rows++
	for k := range obj {
		fields[k] = true
	}
}

// sortedDiff returns the elements in a but not in b, sorted.
func sortedDiff(a, b map[string]bool) []string {
	out := make([]string, 0, len(a))
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// sortedIntersection returns the elements in both a and b, sorted.
func sortedIntersection(a, b map[string]bool) []string {
	out := make([]string, 0)
	for k := range a {
		if b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// WriteText renders r as a human-readable summary on w. Format:
//
//	datalint diff
//	  old: <path>  (<n> rows)
//	  new: <path>  (<n> rows; +/-N)
//	  fields added:    [a, b]
//	  fields removed:  [c]
//	  fields in both:  [x, y, z]
func WriteText(w io.Writer, r Report) error {
	delta := r.NewRows - r.OldRows
	sign := ""
	if delta > 0 {
		sign = "+"
	}
	lines := []string{
		"datalint diff",
		fmt.Sprintf("  old: %s  (%d rows)", r.OldPath, r.OldRows),
		fmt.Sprintf("  new: %s  (%d rows; %s%d)", r.NewPath, r.NewRows, sign, delta),
		fmt.Sprintf("  fields added:    [%s]", strings.Join(r.Added, ", ")),
		fmt.Sprintf("  fields removed:  [%s]", strings.Join(r.Removed, ", ")),
		fmt.Sprintf("  fields in both:  [%s]", strings.Join(r.Common, ", ")),
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}
