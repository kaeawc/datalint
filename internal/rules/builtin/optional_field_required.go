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
// not all rows. Such fields are usually either:
//
//  1. effectively required and the schema is mislabeling them as
//     optional (which downstream consumers will eventually crash on);
//  2. or the few missing rows are real anomalies worth checking.
//
// v0 is presence-ratio-based and does not consult an explicit schema
// declaration. A user-supplied schema declaration is a follow-up.
func checkOptionalFieldRequired(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	minPresence := ctx.Settings.Float("min_presence_ratio", optionalFieldMinPresenceDefault)
	minRows := ctx.Settings.Int("min_rows", optionalFieldMinRowsDefault)
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
	emitOptionalFieldFindings(presence, totalRows, minPresence, path, emit)
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

func emitOptionalFieldFindings(presence map[string]*fieldPresence, totalRows int, minPresence float64, path string, emit func(diag.Finding)) {
	fields := make([]string, 0, len(presence))
	for k := range presence {
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
