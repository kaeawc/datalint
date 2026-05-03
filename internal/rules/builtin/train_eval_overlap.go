package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:          "train-eval-overlap",
		Category:    rules.CategoryLeakage,
		Severity:    diag.SeverityError,
		Confidence:  rules.ConfidenceHigh,
		Fix:         rules.FixNone,
		Needs:       rules.NeedsCorpusScan,
		CorpusCheck: checkTrainEvalOverlap,
	})
}

// promptLoc remembers where in the train corpus a given prompt was
// first seen, so eval-side findings can name an exact citation.
type promptLoc struct {
	path string
	row  int
}

// checkTrainEvalOverlap streams each train file once to build a map of
// normalized prompt -> first (path, row), then streams each eval file
// and emits one finding per row whose prompt is also in the train map.
//
// v0 uses exact match on the JSON field "prompt" with leading/trailing
// whitespace trimmed. MinHash + LSH for near-duplicates is the planned
// follow-up, gated behind NeedsLSH so it stays opt-in.
func checkTrainEvalOverlap(ctx *rules.CorpusContext, emit func(diag.Finding)) {
	if ctx == nil || len(ctx.Train) == 0 || len(ctx.Eval) == 0 {
		return
	}
	train := buildTrainIndex(ctx.Train)
	if len(train) == 0 {
		return
	}
	for _, p := range ctx.Eval {
		_ = scanner.StreamJSONL(p, func(row int, line []byte) error {
			prompt := extractPrompt(line)
			if prompt == "" {
				return nil
			}
			loc, hit := train[prompt]
			if !hit {
				return nil
			}
			emit(diag.Finding{
				RuleID:   "train-eval-overlap",
				Severity: diag.SeverityError,
				Message: fmt.Sprintf(
					"eval prompt also appears in train at %s row %d",
					loc.path, loc.row),
				Location: diag.Location{Path: p, Row: row},
			})
			return nil
		})
	}
}

func buildTrainIndex(paths []string) map[string]promptLoc {
	index := map[string]promptLoc{}
	for _, p := range paths {
		path := p
		_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
			prompt := extractPrompt(line)
			if prompt == "" {
				return nil
			}
			if _, exists := index[prompt]; !exists {
				index[prompt] = promptLoc{path: path, row: row}
			}
			return nil
		})
	}
	return index
}

// extractPrompt returns the trimmed string value of the "prompt" field
// or "" when the row isn't a JSON object, lacks the field, or the
// field isn't a string. Future work: configurable field name.
func extractPrompt(line []byte) string {
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return ""
	}
	v, ok := obj["prompt"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}
