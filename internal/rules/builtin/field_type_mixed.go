package builtin

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "field-type-mixed-across-rows",
		Category:   rules.CategorySchema,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkFieldTypeMixed,
	})
}

// jsonTypeOf returns a coarse type tag for a decoded JSON value.
// null returns "" so callers can skip it; "null + string" is a
// common optional-field pattern, not a schema bug.
func jsonTypeOf(v any) string {
	switch v.(type) {
	case nil:
		return ""
	case bool:
		return "boolean"
	case float64, json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

type fieldStat struct {
	counts   map[string]int
	firstRow map[string]int
}

func checkFieldTypeMixed(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path

	stats := map[string]*fieldStat{}
	err := scanner.StreamJSONL(path, func(row int, line []byte) error {
		recordRow(stats, row, line)
		return nil
	})
	if err != nil {
		// jsonl-malformed-line owns read errors.
		return
	}

	emitMixed(stats, path, emit)
}

// recordRow parses a single JSONL row and updates the per-field stats.
// Parse failures are silently skipped — jsonl-malformed-line owns them.
func recordRow(stats map[string]*fieldStat, row int, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	for k, v := range obj {
		recordValue(stats, row, k, v)
	}
}

func recordValue(stats map[string]*fieldStat, row int, k string, v any) {
	tag := jsonTypeOf(v)
	if tag == "" {
		return
	}
	s, ok := stats[k]
	if !ok {
		s = &fieldStat{
			counts:   map[string]int{},
			firstRow: map[string]int{},
		}
		stats[k] = s
	}
	s.counts[tag]++
	if _, seen := s.firstRow[tag]; !seen {
		s.firstRow[tag] = row
	}
}

func emitMixed(stats map[string]*fieldStat, path string, emit func(diag.Finding)) {
	fields := make([]string, 0, len(stats))
	for k := range stats {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	for _, field := range fields {
		s := stats[field]
		if len(s.counts) < 2 {
			continue
		}
		dominant, dominantCount := pickDominant(s.counts)
		for _, t := range sortedKeys(s.counts) {
			if t == dominant {
				continue
			}
			emit(diag.Finding{
				RuleID:   "field-type-mixed-across-rows",
				Severity: diag.SeverityWarning,
				Message: fmt.Sprintf(
					"field %q has mixed types: %s dominant (%d rows); %s first seen here (%d rows)",
					field, dominant, dominantCount, t, s.counts[t]),
				Location: diag.Location{Path: path, Row: s.firstRow[t]},
			})
		}
	}
}

// pickDominant returns the type with the highest count. Ties are
// broken by lexical order of the type name so output is deterministic.
func pickDominant(counts map[string]int) (string, int) {
	keys := sortedKeys(counts)
	var winner string
	var best int
	for _, k := range keys {
		if counts[k] > best {
			winner = k
			best = counts[k]
		}
	}
	return winner, best
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
