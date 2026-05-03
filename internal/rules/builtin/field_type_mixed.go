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
		if len(line) == 0 {
			return nil
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			// jsonl-malformed-line owns parse errors.
			return nil
		}
		for k, v := range obj {
			tag := jsonTypeOf(v)
			if tag == "" {
				continue
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
		return nil
	})
	if err != nil {
		// jsonl-malformed-line owns read errors.
		return
	}

	emitMixed(stats, path, emit)
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
		types := sortedKeys(s.counts)
		for _, t := range types {
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
