// Package diff compares two JSONL dataset versions and reports
// distribution shifts: row count, field set deltas, and (for shared
// enum-like string fields) the top-K value counts in each version.
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

// MaxDistinctForDistribution caps which fields earn a distribution
// section. Fields with more than this many distinct string values
// (in either old OR new) are treated as free-text and skipped — top
// values aren't informative for them.
const MaxDistinctForDistribution = 20

// TopK is the number of values per side reported in each
// FieldDistribution section.
const TopK = 5

// Report is the diff between an "old" and "new" JSONL file. Counts
// are total parseable rows (malformed rows are not counted).
type Report struct {
	OldPath       string
	NewPath       string
	OldRows       int
	NewRows       int
	Added         []string            // top-level field names present in new but not old
	Removed       []string            // top-level field names present in old but not new
	Common        []string            // top-level field names present in both
	Distributions []FieldDistribution // per shared enum-like string field
}

// FieldDistribution carries the top-K most frequent string values
// for one shared field, in each version. Lists are sorted by count
// descending, ties broken alphabetically for stable output.
type FieldDistribution struct {
	Field  string
	OldTop []ValueCount
	NewTop []ValueCount
}

// ValueCount pairs a string value with its row count.
type ValueCount struct {
	Value string
	Count int
}

// fileStats are the running counts scanFields collects per file.
type fileStats struct {
	fields map[string]bool
	values map[string]map[string]int // field -> value -> count
}

// Compute streams both files once each, recording the set of
// top-level field names, parseable-row count, and per-field per-value
// counts for string-typed values. Malformed rows are silently skipped
// — a separate jsonl-malformed-line lint run is the right place to
// surface them.
func Compute(oldPath, newPath string) (Report, error) {
	oldRows, oldStats, err := scanFields(oldPath)
	if err != nil {
		return Report{}, fmt.Errorf("scan old: %w", err)
	}
	newRows, newStats, err := scanFields(newPath)
	if err != nil {
		return Report{}, fmt.Errorf("scan new: %w", err)
	}
	common := sortedIntersection(oldStats.fields, newStats.fields)
	return Report{
		OldPath:       oldPath,
		NewPath:       newPath,
		OldRows:       oldRows,
		NewRows:       newRows,
		Added:         sortedDiff(newStats.fields, oldStats.fields),
		Removed:       sortedDiff(oldStats.fields, newStats.fields),
		Common:        common,
		Distributions: buildDistributions(common, oldStats.values, newStats.values),
	}, nil
}

func scanFields(path string) (int, fileStats, error) {
	rows := 0
	stats := fileStats{
		fields: map[string]bool{},
		values: map[string]map[string]int{},
	}
	err := scanner.StreamJSONL(path, func(_ int, line []byte) error {
		recordRow(&rows, &stats, line)
		return nil
	})
	return rows, stats, err
}

// recordRow parses one JSONL row and updates the running stats.
// Extracted so the StreamJSONL closure has no error-returning
// surface (nilerr pattern from PR #1 / #7).
func recordRow(rows *int, stats *fileStats, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	*rows++
	for k, v := range obj {
		stats.fields[k] = true
		s, isString := v.(string)
		if !isString {
			continue
		}
		if stats.values[k] == nil {
			stats.values[k] = map[string]int{}
		}
		stats.values[k][s]++
	}
}

// buildDistributions emits a FieldDistribution per shared field
// where neither side blew past MaxDistinctForDistribution. Fields
// with no string occurrences in either version are skipped — the
// value count would be zero on both sides.
func buildDistributions(common []string, oldVals, newVals map[string]map[string]int) []FieldDistribution {
	out := make([]FieldDistribution, 0)
	for _, field := range common {
		o := oldVals[field]
		n := newVals[field]
		if len(o) == 0 && len(n) == 0 {
			continue
		}
		distinct := unionSize(o, n)
		if distinct > MaxDistinctForDistribution {
			continue
		}
		out = append(out, FieldDistribution{
			Field:  field,
			OldTop: topByCount(o, TopK),
			NewTop: topByCount(n, TopK),
		})
	}
	return out
}

// topByCount returns the top-k entries from m sorted by count
// descending, ties broken alphabetically for stable output. Returns
// nil for empty input so the section can omit "old: []" / "new: []".
func topByCount(m map[string]int, k int) []ValueCount {
	if len(m) == 0 {
		return nil
	}
	out := make([]ValueCount, 0, len(m))
	for v, c := range m {
		out = append(out, ValueCount{Value: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

func unionSize(a, b map[string]int) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	return len(seen)
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

// WriteJSON marshals the Report as a pretty-printed JSON object so
// scripted consumers (CI scrapers, dashboards) can parse it the same
// way they parse `--format=json` findings. Field names follow the Go
// struct via standard json marshaling — capitalized keys.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText renders r as a human-readable summary on w. The trailing
// distribution section appears only when at least one shared
// enum-like string field showed up.
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
	if len(r.Distributions) > 0 {
		lines = append(lines, "  field distributions (shared string fields, ≤"+fmt.Sprintf("%d", MaxDistinctForDistribution)+" distinct values):")
		for _, fd := range r.Distributions {
			lines = append(lines, fmt.Sprintf("    %s:", fd.Field))
			lines = append(lines, fmt.Sprintf("      old top: %s", formatValueCounts(fd.OldTop)))
			lines = append(lines, fmt.Sprintf("      new top: %s", formatValueCounts(fd.NewTop)))
		}
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func formatValueCounts(vc []ValueCount) string {
	if len(vc) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(vc))
	for _, v := range vc {
		parts = append(parts, fmt.Sprintf("%s:%d", v.Value, v.Count))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
