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
	"math"
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

// FieldDistribution carries the top-K most frequent string values,
// character-length stats, and Unicode-script mix for one shared
// field, in each version. Top lists are sorted by count descending,
// ties broken alphabetically for stable output. LengthStats are
// computed over every string occurrence of the field (not just
// top-K). Scripts are sorted by letter count descending; both sides
// are nil when neither version has enough letters to report a stable
// mix (MinRunesForScriptMix).
type FieldDistribution struct {
	Field      string
	OldTop     []ValueCount
	NewTop     []ValueCount
	OldLength  LengthStats
	NewLength  LengthStats
	OldScripts []ScriptCount
	NewScripts []ScriptCount
}

// ScriptCount pairs a Unicode-script name (e.g. "Latin", "Han",
// "Cyrillic"; "Other" for letters that don't match a tracked
// script) with its letter count and the ratio against the per-side
// total of tracked-script letters. Non-letter runes (digits,
// punctuation, whitespace) are not included in the denominator.
type ScriptCount struct {
	Script string
	Count  int
	Ratio  float64
}

// LengthStats summarises character-length distribution per field.
// Count is the number of string occurrences; the rest are computed
// only when Count > 0. Min and Max are the bounds of the sorted
// occurrences. Percentiles use linear interpolation against the
// sorted vector (rank = p * (N-1); result blends the floor and ceil
// elements by the fractional rank), so P50 of [4, 6] is 5.0 rather
// than the nearest-rank 4 or 6.
type LengthStats struct {
	Count int
	Mean  float64
	Min   int
	P50   float64
	P90   float64
	P99   float64
	Max   int
}

// ValueCount pairs a string value with its row count.
type ValueCount struct {
	Value string
	Count int
}

// fileStats are the running counts scanFields collects per file.
// lengths records every string occurrence's character length so the
// final pass can compute percentiles per field. scripts accumulates
// per-field per-script letter counts across all rows for the
// language-mix section.
type fileStats struct {
	fields  map[string]bool
	values  map[string]map[string]int // field -> value -> count
	lengths map[string][]int          // field -> per-occurrence char lengths
	scripts map[string]map[string]int // field -> script name -> letter count
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
		Distributions: buildDistributions(common, oldStats, newStats),
	}, nil
}

func scanFields(path string) (int, fileStats, error) {
	rows := 0
	stats := fileStats{
		fields:  map[string]bool{},
		values:  map[string]map[string]int{},
		lengths: map[string][]int{},
		scripts: map[string]map[string]int{},
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
		stats.lengths[k] = append(stats.lengths[k], len(s))
		mergeScriptCounts(stats.scripts, k, countScripts(s))
	}
}

// mergeScriptCounts accumulates per-rune script counts for one
// string into the running per-field tally on stats.scripts.
func mergeScriptCounts(dest map[string]map[string]int, field string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	if dest[field] == nil {
		dest[field] = map[string]int{}
	}
	for name, c := range counts {
		dest[field][name] += c
	}
}

// buildDistributions emits a FieldDistribution per shared field
// that has at least one string occurrence on either side. The
// per-section gating differs:
//
//   - Top values are populated only when the union cardinality is
//     ≤ MaxDistinctForDistribution. Free-text fields would
//     otherwise produce a noisy "top 5 of thousands" list.
//   - Length stats are always populated when string occurrences
//     exist. They're useful even for free-text fields ("prompts
//     grew 3× longer on average").
//   - Script mix is populated only when at least one side has
//     ≥ MinRunesForScriptMix tracked letters; small enum fields
//     don't produce a stable script profile.
func buildDistributions(common []string, oldStats, newStats fileStats) []FieldDistribution {
	out := make([]FieldDistribution, 0)
	for _, field := range common {
		o := oldStats.values[field]
		n := newStats.values[field]
		if len(o) == 0 && len(n) == 0 {
			continue
		}
		oldScripts, newScripts := buildScriptMix(oldStats.scripts[field], newStats.scripts[field])
		fd := FieldDistribution{
			Field:      field,
			OldLength:  computeLengthStats(oldStats.lengths[field]),
			NewLength:  computeLengthStats(newStats.lengths[field]),
			OldScripts: oldScripts,
			NewScripts: newScripts,
		}
		if unionSize(o, n) <= MaxDistinctForDistribution {
			fd.OldTop = topByCount(o, TopK)
			fd.NewTop = topByCount(n, TopK)
		}
		out = append(out, fd)
	}
	return out
}

// computeLengthStats sorts a copy of lengths and computes mean,
// min, max, and the linearly-interpolated p50 / p90 / p99. Caller
// should not mutate the returned struct's state — it's a value, but
// this is a friendly note.
func computeLengthStats(lengths []int) LengthStats {
	if len(lengths) == 0 {
		return LengthStats{}
	}
	sorted := make([]int, len(lengths))
	copy(sorted, lengths)
	sort.Ints(sorted)
	sum := 0
	for _, n := range sorted {
		sum += n
	}
	return LengthStats{
		Count: len(sorted),
		Mean:  float64(sum) / float64(len(sorted)),
		Min:   sorted[0],
		P50:   interpolatedPercentile(sorted, 0.50),
		P90:   interpolatedPercentile(sorted, 0.90),
		P99:   interpolatedPercentile(sorted, 0.99),
		Max:   sorted[len(sorted)-1],
	}
}

// interpolatedPercentile returns p (in [0, 1]) of sorted using the
// linear-interpolation method: rank = p * (N-1), then blend the
// floor and ceil elements by the fractional rank. For N = 1 it
// degenerates to the single value. Caller passes a sorted slice.
func interpolatedPercentile(sorted []int, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return float64(sorted[0])
	}
	rank := p * float64(len(sorted)-1)
	low := int(math.Floor(rank))
	high := int(math.Ceil(rank))
	if low == high {
		return float64(sorted[low])
	}
	frac := rank - float64(low)
	return float64(sorted[low]) + frac*float64(sorted[high]-sorted[low])
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
// distribution section appears only when at least one shared string
// field showed up. Each subsection (top values, length stats, script
// mix) is rendered only for fields where it carries signal — top
// values are skipped on free-text fields, script mix is skipped on
// short enum fields.
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
		lines = append(lines, "  field distributions (shared string fields):")
		for _, fd := range r.Distributions {
			lines = append(lines, formatFieldDistribution(fd)...)
		}
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

// formatFieldDistribution renders one FieldDistribution as a slice
// of lines (without the trailing newline) for WriteText. Each
// subsection is gated on having something useful to say so free-
// text and enum fields don't emit empty "old top: []" lines.
func formatFieldDistribution(fd FieldDistribution) []string {
	out := []string{fmt.Sprintf("    %s:", fd.Field)}
	if len(fd.OldTop) > 0 || len(fd.NewTop) > 0 {
		out = append(out,
			fmt.Sprintf("      old top:     %s", formatValueCounts(fd.OldTop)),
			fmt.Sprintf("      new top:     %s", formatValueCounts(fd.NewTop)),
		)
	}
	if fd.OldLength.Count > 0 {
		out = append(out, fmt.Sprintf("      old length:  %s", formatLengthStats(fd.OldLength)))
	}
	if fd.NewLength.Count > 0 {
		out = append(out, fmt.Sprintf("      new length:  %s", formatLengthStats(fd.NewLength)))
	}
	if len(fd.OldScripts) > 0 || len(fd.NewScripts) > 0 {
		out = append(out,
			fmt.Sprintf("      old scripts: %s", formatScriptMix(fd.OldScripts)),
			fmt.Sprintf("      new scripts: %s", formatScriptMix(fd.NewScripts)),
		)
	}
	return out
}

func formatLengthStats(s LengthStats) string {
	return fmt.Sprintf("count=%d mean=%.1f min=%d p50=%.1f p90=%.1f p99=%.1f max=%d",
		s.Count, s.Mean, s.Min, s.P50, s.P90, s.P99, s.Max)
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

func formatScriptMix(sc []ScriptCount) string {
	if len(sc) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(sc))
	for _, s := range sc {
		parts = append(parts, fmt.Sprintf("%s:%d (%.0f%%)", s.Script, s.Count, s.Ratio*100))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
