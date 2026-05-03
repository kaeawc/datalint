// Package builtin registers the rules shipped in the datalint binary.
// Importing this package for side effects pulls in every built-in rule.
package builtin

import (
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
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

func checkJSONLMalformed(_ *rules.Context, _ func(diag.Finding)) {
	// TODO: stream the JSONL file via internal/scanner, emit one
	// Finding per non-parseable line with the offending row index.
}
