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
		ID:         "optional-field-required-by-downstream",
		Category:   rules.CategorySchema,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkOptionalFieldRequired,
	})
}

// optionalFieldMinPresenceDefault is the lower bound on the
// "almost-always present" ratio that flags a field. The rule fires
// for ratios in [min_presence_ratio, 1.0) — strict 100% means
// "consistently required" (no anomaly), so it's deliberately
// excluded from the band.
//
// optionalFieldMinRowsDefault keeps the rule quiet on small samples
// where presence ratios are noisy. Production users will likely
// raise both:
//
//	rules:
//	  optional-field-required-by-downstream:
//	    min_presence_ratio: 0.95
//	    min_rows: 50
const (
	optionalFieldMinPresenceDefault = 0.8
	optionalFieldMinRowsDefault     = 5
)

// fieldPresence tracks per-field stats across a single file.
type fieldPresence struct {
	seenIn          int
	firstMissingRow int
}

// checkOptionalFieldRequired flags fields that appear in most but
// not all rows. Two paths:
//
//  1. Explicit schema. When the user declares a `required_fields`
//     list in config, every row must contain every declared field.
//     A field with any missing rows fires a finding citing the
//     missing count. Fields covered by the schema are skipped by
//     the heuristic path so they don't double-fire.
//
//  2. Presence-ratio heuristic. For fields not covered by an
//     explicit schema, fire when a field is present in ≥
//     min_presence_ratio but < 100% of rows. The mid-band is
//     usually either a mislabeled-optional field downstream
//     consumers will eventually crash on, or a few real anomalies
//     worth checking.
func checkOptionalFieldRequired(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	minPresence := ctx.Settings.Float("min_presence_ratio", optionalFieldMinPresenceDefault)
	minRows := ctx.Settings.Int("min_rows", optionalFieldMinRowsDefault)
	requiredFields := ctx.Settings.StringSlice("required_fields")
	path := ctx.File.Path

	presence := map[string]*fieldPresence{}
	totalRows := 0

	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		recordOptionalFieldRow(presence, &totalRows, row, line)
		return nil
	})

	if totalRows < minRows {
		return
	}
	emitSchemaRequiredFindings(presence, totalRows, requiredFields, path, emit)
	emitOptionalFieldFindings(presence, totalRows, minPresence, requiredFields, path, emit)
}

func recordOptionalFieldRow(presence map[string]*fieldPresence, totalRows *int, row int, line []byte) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	*totalRows++

	seenHere := map[string]bool{}
	for k := range obj {
		seenHere[k] = true
	}

	// For tracked fields, count presence/absence.
	for field, p := range presence {
		if seenHere[field] {
			p.seenIn++
			continue
		}
		if p.firstMissingRow == 0 {
			p.firstMissingRow = row
		}
	}

	// New fields seen for the first time. If they appeared on row
	// 2+, they were absent in earlier rows — record row 1 as the
	// first missing row.
	for field := range seenHere {
		if _, exists := presence[field]; exists {
			continue
		}
		p := &fieldPresence{seenIn: 1}
		if *totalRows > 1 {
			p.firstMissingRow = 1
		}
		presence[field] = p
	}
}

// emitSchemaRequiredFindings fires for each declared-required
// field that's missing on any row. A field declared required but
// never present at all gets a single finding pointing at row 1.
func emitSchemaRequiredFindings(presence map[string]*fieldPresence, totalRows int, requiredFields []string, path string, emit func(diag.Finding)) {
	if len(requiredFields) == 0 {
		return
	}
	sorted := append([]string(nil), requiredFields...)
	sort.Strings(sorted)
	for _, field := range sorted {
		p, ok := presence[field]
		missing := totalRows
		firstMissingRow := 1
		if ok {
			missing = totalRows - p.seenIn
			if p.firstMissingRow != 0 {
				firstMissingRow = p.firstMissingRow
			}
		}
		if missing == 0 {
			continue
		}
		emit(diag.Finding{
			RuleID:   "optional-field-required-by-downstream",
			Severity: diag.SeverityWarning,
			Message: fmt.Sprintf(
				"schema declares %q as required but %d/%d rows are missing it",
				field, missing, totalRows),
			Location: diag.Location{Path: path, Row: firstMissingRow},
		})
	}
}

func emitOptionalFieldFindings(presence map[string]*fieldPresence, totalRows int, minPresence float64, requiredFields []string, path string, emit func(diag.Finding)) {
	skip := make(map[string]bool, len(requiredFields))
	for _, f := range requiredFields {
		skip[f] = true
	}
	fields := make([]string, 0, len(presence))
	for k := range presence {
		if skip[k] {
			continue
		}
		fields = append(fields, k)
	}
	sort.Strings(fields)

	for _, field := range fields {
		p := presence[field]
		ratio := float64(p.seenIn) / float64(totalRows)
		if ratio < minPresence || ratio >= 1.0 {
			continue
		}
		emit(diag.Finding{
			RuleID:   "optional-field-required-by-downstream",
			Severity: diag.SeverityWarning,
			Message: fmt.Sprintf(
				"field %q appears in %d/%d rows (%.1f%%); either mark it required or check the missing rows",
				field, p.seenIn, totalRows, ratio*100),
			Location: diag.Location{Path: path, Row: p.firstMissingRow},
		})
	}
}
