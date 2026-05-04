package builtin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// schemaMismatch tracks per-(path, observedType) misses across the
// whole file: how many element-level mismatches occurred and the
// first row where one was seen. For array-bearing paths a single
// row can contribute multiple element-level mismatches.
type schemaMismatch struct {
	count    int
	firstRow int
}

// pathSegment is one piece of a parsed `field_types` key. A
// non-array segment carries a literal field name; an array segment
// matches "for each element of this array, evaluate the rest of
// the path".
type pathSegment struct {
	name    string
	isArray bool
}

// parsePath turns a key from `field_types` into a sequence of
// segments. Recognised tokens: alphanumeric runs (field names),
// "." (object-key separator), and "[]" (array-each-element
// marker). Examples:
//
//	"input"               → [name=input]
//	"meta.author"         → [name=meta] [name=author]
//	"messages[]"          → [name=messages] [array]
//	"messages[].role"     → [name=messages] [array] [name=role]
//
// Any other character is appended to the current name run; the
// rule's job is data-quality, not key-syntax validation.
func parsePath(s string) []pathSegment {
	var out []pathSegment
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, pathSegment{name: cur.String()})
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			flush()
			continue
		}
		if c == '[' && i+1 < len(s) && s[i+1] == ']' {
			flush()
			out = append(out, pathSegment{isArray: true})
			i++ // skip ']'
			continue
		}
		cur.WriteByte(c)
	}
	flush()
	return out
}

// evaluatePath descends into v according to segs and returns every
// concrete value the path resolves to. An array segment fans out to
// each element. A missing key or a type mismatch with the path
// shape (e.g. asking for [].role when messages is a string) yields
// no resolved values — the rule treats those as "this path doesn't
// apply to this row" rather than firing.
func evaluatePath(v any, segs []pathSegment) []any {
	if len(segs) == 0 {
		return []any{v}
	}
	seg := segs[0]
	rest := segs[1:]
	if seg.isArray {
		arr, ok := v.([]any)
		if !ok {
			return nil
		}
		var out []any
		for _, e := range arr {
			out = append(out, evaluatePath(e, rest)...)
		}
		return out
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	next, present := obj[seg.name]
	if !present {
		return nil
	}
	return evaluatePath(next, rest)
}

// compiledEntry caches the parsed segments for one declared path so
// recordSchemaTypeRow doesn't re-parse on every row.
type compiledEntry struct {
	path     string
	segments []pathSegment
	declared string
}

// checkFieldTypeMismatchWithSchema fires when a value's actual JSON
// type doesn't match the type declared in the rule's `field_types`
// map. Keys may be top-level field names ("input"), nested paths
// ("meta.author"), array-each-element markers ("messages[]"), or
// nested combinations ("messages[].role"). Missing paths are
// ignored — that's optional-field-required-by-downstream's concern.
// Fields not declared in `field_types` are also ignored; declaring
// a partial schema is fine.
func checkFieldTypeMismatchWithSchema(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	compiled := compileSchema(ctx.Settings.StringMap("field_types"))
	if len(compiled) == 0 {
		return
	}
	path := ctx.File.Path
	mismatches := map[string]map[string]*schemaMismatch{}

	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		recordSchemaTypeRow(compiled, mismatches, row, line)
		return nil
	})

	emitSchemaTypeMismatches(compiled, mismatches, path, emit)
}

// compileSchema parses each `field_types` key as a path and drops
// entries whose declared type isn't one of the supported tags. A
// typo like `field: integer` or an unparseable path is silently
// ignored rather than raising an error — the rule's purpose is
// data-quality, not config-validation. Output is sorted by path
// for deterministic iteration order.
func compileSchema(declared map[string]string) []compiledEntry {
	if len(declared) == 0 {
		return nil
	}
	keys := make([]string, 0, len(declared))
	for k := range declared {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]compiledEntry, 0, len(declared))
	for _, k := range keys {
		t := declared[k]
		if !validSchemaTypes[t] {
			continue
		}
		segs := parsePath(k)
		if len(segs) == 0 {
			continue
		}
		out = append(out, compiledEntry{
			path:     k,
			segments: segs,
			declared: t,
		})
	}
	return out
}

// recordSchemaTypeRow walks one row's parsed object, evaluating
// each compiled path and comparing every resolved value's type
// against the declared type. A path that resolves to multiple
// values (because it crosses an array-each-element marker)
// contributes one count per element. Mismatches are bucketed by
// (path, observed type) so a single finding summarises N elements
// with the same observed mismatch.
func recordSchemaTypeRow(compiled []compiledEntry, mismatches map[string]map[string]*schemaMismatch, row int, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	for _, entry := range compiled {
		resolved := evaluatePath(any(obj), entry.segments)
		for _, v := range resolved {
			got := schemaTypeOf(v)
			if got == entry.declared {
				continue
			}
			recordSchemaMismatch(mismatches, entry.path, got, row)
		}
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

func emitSchemaTypeMismatches(compiled []compiledEntry, mismatches map[string]map[string]*schemaMismatch, path string, emit func(diag.Finding)) {
	declared := make(map[string]string, len(compiled))
	for _, e := range compiled {
		declared[e.path] = e.declared
	}
	keys := make([]string, 0, len(mismatches))
	for k := range mismatches {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		want := declared[key]
		seen := mismatches[key]
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
					"path %q declared as %s but observed as %s in %d value(s); first at row %d",
					key, want, observed, m.count, m.firstRow),
				Location: diag.Location{Path: path, Row: m.firstRow},
			})
		}
	}
}
