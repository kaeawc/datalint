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
		ID:         "enum-drift",
		Category:   rules.CategorySchema,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkEnumDrift,
	})
}

// enumLockInRows / enumMaxDistinct are the v0 thresholds. After
// enumLockInRows occurrences of a string field the rule "locks in"
// its observed value set; subsequent rows that introduce values
// outside that set are flagged. Fields whose lock-in set already
// holds more than enumMaxDistinct values are treated as free-text
// (notEnum) and never flagged. Defaults are very sensitive so a
// 6-row fixture exercises the rule end-to-end; production users
// will want to raise both via the (not-yet-implemented) config.
const (
	enumLockInRows  = 5
	enumMaxDistinct = 8
)

// enumStats is the per-field running state across a single file.
type enumStats struct {
	values   map[string]int // value -> first row seen
	rowCount int            // rows processed where the field had a string value
	locked   bool
	notEnum  bool
	flagged  map[string]bool // values already emitted, dedupe within file
}

func checkEnumDrift(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	stats := map[string]*enumStats{}
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		recordEnumRow(stats, path, row, line, emit)
		return nil
	})
}

func recordEnumRow(stats map[string]*enumStats, path string, row int, line []byte, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	// Stable iteration order so the lock-in row counts deterministically
	// when more than one field is present.
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s, ok := obj[k].(string)
		if !ok {
			continue
		}
		recordEnumValue(stats, path, row, k, s, emit)
	}
}

func recordEnumValue(stats map[string]*enumStats, path string, row int, field, value string, emit func(diag.Finding)) {
	e, ok := stats[field]
	if !ok {
		e = &enumStats{
			values:  map[string]int{},
			flagged: map[string]bool{},
		}
		stats[field] = e
	}

	if !e.locked {
		if _, exists := e.values[value]; !exists {
			e.values[value] = row
		}
		e.rowCount++
		if e.rowCount >= enumLockInRows {
			e.locked = true
			if len(e.values) > enumMaxDistinct {
				e.notEnum = true
			}
		}
		return
	}
	if e.notEnum {
		return
	}
	if _, exists := e.values[value]; exists {
		return
	}
	if e.flagged[value] {
		return
	}
	e.flagged[value] = true
	emit(diag.Finding{
		RuleID:   "enum-drift",
		Severity: diag.SeverityWarning,
		Message: fmt.Sprintf(
			"field %q gained value %q not seen in the first %d lock-in rows",
			field, value, enumLockInRows),
		Location: diag.Location{Path: path, Row: row},
	})
}
