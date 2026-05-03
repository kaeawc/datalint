// Package builtin registers the rules shipped in the datalint binary.
// Importing this package for side effects pulls in every built-in rule.
package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "jsonl-malformed-line",
		Category:   rules.CategoryFile,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkJSONLMalformed,
	})
}

func checkJSONLMalformed(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	err := scanner.StreamJSONL(path, func(row int, line []byte) error {
		if len(bytes.TrimSpace(line)) == 0 {
			emit(diag.Finding{
				RuleID:   "jsonl-malformed-line",
				Severity: diag.SeverityError,
				Message:  "blank line in JSONL",
				Location: diag.Location{Path: path, Row: row},
			})
			return nil
		}
		var v any
		if jsonErr := json.Unmarshal(line, &v); jsonErr != nil {
			emit(diag.Finding{
				RuleID:   "jsonl-malformed-line",
				Severity: diag.SeverityError,
				Message:  fmt.Sprintf("invalid JSON: %v", jsonErr),
				Location: diag.Location{Path: path, Row: row},
			})
		}
		return nil
	})
	if err != nil {
		emit(diag.Finding{
			RuleID:   "jsonl-malformed-line",
			Severity: diag.SeverityError,
			Message:  fmt.Sprintf("read error: %v", err),
			Location: diag.Location{Path: path},
		})
	}
}
