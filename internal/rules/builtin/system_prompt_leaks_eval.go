package builtin

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "system-prompt-leaks-eval-instructions",
		Category:   rules.CategoryLeakage,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkSystemPromptLeaksEval,
	})
}

// builtinEvalInstructionPatterns are the canonical eval-prompt
// phrases datalint flags out of the box. Patterns are
// case-insensitive (?i). Users add project-specific phrasings via
// datalint.yml:
//
//	rules:
//	  system-prompt-leaks-eval-instructions:
//	    extra_patterns:
//	      - "(?i)reply with one of"
//	      - "MMLU"
var builtinEvalInstructionPatterns = mustCompileAll([]string{
	`(?i)respond (only )?with (one of|the letter|just|a single)`,
	`(?i)answer (only )?with (a|the) (letter|option|choice)`,
	`(?i)the correct answer is`,
	`(?i)choose (from|one of|between) the (following|options|choices)`,
	`(?i)select (your answer|the correct) from`,
	`(?i)from the following options`,
	`(?i)answer in the format`,
	`(?i)pick (one of|the correct|the best)`,
})

// sysContentMsg is the minimal shape we need: role and a string
// content. Content arrays (Anthropic-style structured content) are
// ignored — they're a follow-up.
type sysContentMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sysContentRow struct {
	Messages []sysContentMsg `json:"messages"`
}

// checkSystemPromptLeaksEval flags rows whose any system message
// contains a phrase that looks like an eval instruction. Eval
// instructions in training-data system prompts force the model to
// learn the eval's response shape, contaminating downstream metrics.
//
// One finding per row, anchored at the first offending system
// message and naming the matched substring so reviewers can decide
// whether it's a real leak or an in-distribution instruction.
func checkSystemPromptLeaksEval(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	patterns := patternsFromSettings(ctx.Settings)
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkSystemPromptLeaksRow(path, row, line, patterns, emit)
		return nil
	})
}

func checkSystemPromptLeaksRow(path string, row int, line []byte, patterns []*regexp.Regexp, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var r sysContentRow
	if json.Unmarshal(line, &r) != nil {
		return
	}
	idx, match, ok := firstLeakingSystem(r.Messages, patterns)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "system-prompt-leaks-eval-instructions",
		Severity: diag.SeverityWarning,
		Message: fmt.Sprintf(
			"system message at index %d matches eval-instruction pattern %q",
			idx, match),
		Location: diag.Location{Path: path, Row: row},
	})
}

func firstLeakingSystem(msgs []sysContentMsg, patterns []*regexp.Regexp) (int, string, bool) {
	for i, m := range msgs {
		if m.Role != "system" || m.Content == "" {
			continue
		}
		for _, pat := range patterns {
			if loc := pat.FindStringIndex(m.Content); loc != nil {
				return i, m.Content[loc[0]:loc[1]], true
			}
		}
	}
	return 0, "", false
}

// patternsFromSettings builds the regex set per call. Built-in
// patterns are static; extra_patterns from datalint.yml are appended.
// Bad regex entries are skipped; a future config-validation pass
// will surface them.
func patternsFromSettings(s config.RuleConfig) []*regexp.Regexp {
	patterns := builtinEvalInstructionPatterns
	for _, raw := range s.StringSlice("extra_patterns") {
		re, err := regexp.Compile(raw)
		if err != nil {
			continue
		}
		patterns = append(patterns, re)
	}
	return patterns
}

func mustCompileAll(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}
