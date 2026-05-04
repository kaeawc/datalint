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
		ID:         "field-type-mismatch-with-schema",
		Category:   rules.CategorySchema,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkFieldTypeMismatchWithSchema,
	})
}

// validSchemaTypes are the type tags datalint accepts in
// `field_types`. They mirror the JSON value space (no float vs int
// distinction; "number" covers both). "null" is accepted so users
// can declare nullable fields explicitly, e.g. score: null when the
// schema permits a null entry.
var validSchemaTypes = map[string]bool{
	"string":  true,
	"number":  true,
	"boolean": true,
	"array":   true,
	"object":  true,
	"null":    true,
}

// schemaMismatch tracks per-(field, observedType) misses across the
// whole file: how many rows it occurred on and the first such row.
type schemaMismatch struct {
	count    int
	firstRow int
}

// checkFieldTypeMismatchWithSchema fires when a field's actual JSON
// type doesn't match the type declared in the rule's `field_types`
// map. Missing fields are ignored — that's optional-field-required-
// by-downstream's concern. Fields not declared in `field_types` are
// also ignored — declaring a partial schema is fine.
func checkFieldTypeMismatchWithSchema(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	declared := normalisedSchema(ctx.Settings.StringMap("field_types"))
	if len(declared) == 0 {
		return
	}
	path := ctx.File.Path
	mismatches := map[string]map[string]*schemaMismatch{}

	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		recordSchemaTypeRow(declared, mismatches, row, line)
		return nil
	})

	emitSchemaTypeMismatches(declared, mismatches, path, emit)
}

// normalisedSchema drops entries whose declared type isn't one of
// the supported tags. A typo like `field: integer` is silently
// ignored rather than raising an error — the rule's purpose is
// data-quality, not config-validation.
func normalisedSchema(declared map[string]string) map[string]string {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]string, len(declared))
	for k, v := range declared {
		if validSchemaTypes[v] {
			out[k] = v
		}
	}
	return out
}

// recordSchemaTypeRow walks one row, comparing each declared field
// against the observed value type. Missing fields are skipped (the
// optional-field rule covers those). Mismatches are bucketed by
// (field, observed) so a single finding can summarise N rows with
// the same observed type.
func recordSchemaTypeRow(declared map[string]string, mismatches map[string]map[string]*schemaMismatch, row int, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	for field, want := range declared {
		v, present := obj[field]
		if !present {
			continue
		}
		got := schemaTypeOf(v)
		if got == want {
			continue
		}
		recordSchemaMismatch(mismatches, field, got, row)
	}
}

// schemaTypeOf is jsonTypeOf with explicit null handling — for
// schema validation, null is its own type, not a "skip" signal.
func schemaTypeOf(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
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
	}
	return "unknown"
}

func recordSchemaMismatch(mismatches map[string]map[string]*schemaMismatch, field, observed string, row int) {
	if mismatches[field] == nil {
		mismatches[field] = map[string]*schemaMismatch{}
	}
	m, ok := mismatches[field][observed]
	if !ok {
		mismatches[field][observed] = &schemaMismatch{count: 1, firstRow: row}
		return
	}
	m.count++
}

func emitSchemaTypeMismatches(declared map[string]string, mismatches map[string]map[string]*schemaMismatch, path string, emit func(diag.Finding)) {
	fields := make([]string, 0, len(mismatches))
	for k := range mismatches {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	for _, field := range fields {
		want := declared[field]
		seen := mismatches[field]
		observedTypes := make([]string, 0, len(seen))
		for t := range seen {
			observedTypes = append(observedTypes, t)
		}
		sort.Strings(observedTypes)
		for _, observed := range observedTypes {
			m := seen[observed]
			emit(diag.Finding{
				RuleID:   "field-type-mismatch-with-schema",
				Severity: diag.SeverityWarning,
				Message: fmt.Sprintf(
					"field %q declared as %s but observed as %s in %d row(s); first at row %d",
					field, want, observed, m.count, m.firstRow),
				Location: diag.Location{Path: path, Row: m.firstRow},
			})
		}
	}
}
