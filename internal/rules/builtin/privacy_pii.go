package builtin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "privacy-pii-detected",
		Category:   rules.CategoryFile,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkPrivacyPII,
	})
}

// piiPattern pairs a name (cited in the finding message) with its
// compiled regex. Names are lower-case and stable so consumers can
// filter on them.
type piiPattern struct {
	name string
	re   *regexp.Regexp
}

// builtinPIIPatterns are the patterns datalint flags out of the box.
// Conservative — false positives in this category are far less
// damaging than false negatives, but a few obvious anchors keep the
// signal honest:
//
//   - email: requires a TLD with at least 2 letters.
//   - us-ssn: requires the canonical 3-2-4 dashed form.
//   - phone: requires either an explicit country code OR a 3-3-4
//     dashed/parenthesized layout to avoid catching every number
//     sequence.
//   - credit-card: 13–19 digits with optional grouping spaces/dashes;
//     conservative enough that it won't catch order numbers.
var builtinPIIPatterns = []piiPattern{
	{"email", regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)},
	{"us-ssn", regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
	{"phone", regexp.MustCompile(`(?:\+\d{1,3}[ \-]?)?\(?\d{3}\)?[ \-]\d{3}[ \-]\d{4}`)},
	{"credit-card", regexp.MustCompile(`\b\d{4}[ \-]\d{4}[ \-]\d{4}[ \-]\d{4}\b`)},
}

// checkPrivacyPII scans every string value of every JSONL row for
// PII patterns. Emits one finding per row even if multiple patterns
// match — keeps output proportional to the offending row count, not
// the field count.
func checkPrivacyPII(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	patterns := piiPatternsFromSettings(ctx.Settings)
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkPrivacyPIIRow(path, row, line, patterns, emit)
		return nil
	})
}

func checkPrivacyPIIRow(path string, row int, line []byte, patterns []piiPattern, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	field, kind, sample, ok := firstPIIMatch(obj, patterns)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "privacy-pii-detected",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"%s pattern matched in field %q (sample: %q); training data should not carry user PII",
			kind, field, sample),
		Location: diag.Location{Path: path, Row: row},
	})
}

// firstPIIMatch walks the row's top-level string fields in sorted
// order so the per-row finding is deterministic, returning the first
// (field, pattern, matched-substring) hit.
func firstPIIMatch(obj map[string]any, patterns []piiPattern) (field, kind, sample string, ok bool) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s, isString := obj[k].(string)
		if !isString {
			continue
		}
		for _, p := range patterns {
			if loc := p.re.FindStringIndex(s); loc != nil {
				return k, p.name, s[loc[0]:loc[1]], true
			}
		}
	}
	return "", "", "", false
}

// piiPatternsFromSettings appends user-supplied patterns from
// extra_patterns to the built-in list. Each entry must include a
// name= prefix so the finding message can cite it; entries that
// can't compile are silently skipped (a future config-validation
// pass will surface them).
//
//	rules:
//	  privacy-pii-detected:
//	    extra_patterns:
//	      - "internal-id=INT-\\d{6,}"
//	      - "(?i)passport: ?[A-Z0-9]{6,9}"
func piiPatternsFromSettings(s config.RuleConfig) []piiPattern {
	patterns := builtinPIIPatterns
	for _, raw := range s.StringSlice("extra_patterns") {
		name, expr := splitExtraPattern(raw)
		re, err := regexp.Compile(expr)
		if err != nil {
			continue
		}
		patterns = append(patterns, piiPattern{name: name, re: re})
	}
	return patterns
}

// splitExtraPattern accepts either "name=regex" or a bare regex. The
// fallback name is "custom" so the finding still has something to
// cite.
func splitExtraPattern(raw string) (name, expr string) {
	for i := 0; i < len(raw); i++ {
		if raw[i] == '=' {
			return raw[:i], raw[i+1:]
		}
	}
	return "custom", raw
}
