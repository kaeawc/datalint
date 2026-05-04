package builtin

import (
	"fmt"
	"sort"

	"github.com/kaeawc/datalint/internal/dedup"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:          "cross-dataset-overlap",
		Category:    rules.CategoryLeakage,
		Severity:    diag.SeverityError,
		Confidence:  rules.ConfidenceHigh,
		Fix:         rules.FixNone,
		Needs:       rules.NeedsCorpusScan,
		CorpusCheck: checkCrossDatasetOverlap,
	})
}

// checkCrossDatasetOverlap is the N-way generalization of
// train-eval-overlap. For each ordered pair (a, b) of datasets where
// a < b lexically, it builds the train index for a (exact map +
// optional MinHash signatures + LSH) and streams b's rows against
// it, emitting a finding on b's row when a prompt matches. Same
// config knobs as train-eval-overlap (prompt_field,
// near_dup_threshold) — they live under this rule's own config key.
//
// The pairwise direction (a → b, not b → a) is intentional: every
// overlap surfaces exactly once, anchored at the lexically-later
// dataset. Avoids twice-emitting the same overlap from both sides.
func checkCrossDatasetOverlap(ctx *rules.CorpusContext, emit func(diag.Finding)) {
	if ctx == nil || len(ctx.Datasets) < 2 {
		return
	}
	field := ctx.Settings.String("prompt_field", promptFieldDefault)
	threshold := ctx.Settings.Float("near_dup_threshold", 0.0)

	var mh *dedup.MinHash
	if threshold > 0 {
		mh = dedup.New(0)
	}

	names := sortedDatasetNames(ctx.Datasets)
	indices := make(map[string]trainIndex, len(names))
	for _, name := range names {
		indices[name] = buildTrainIndex(ctx.Datasets[name], field, mh)
	}

	for i, a := range names {
		for _, b := range names[i+1:] {
			emitPairwiseOverlap(a, b, indices[a], ctx.Datasets[b], field, threshold, mh, emit)
		}
	}
}

func sortedDatasetNames(d map[string][]string) []string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func emitPairwiseOverlap(aName, bName string, aIdx trainIndex, bPaths []string, field string, threshold float64, mh *dedup.MinHash, emit func(diag.Finding)) {
	for _, p := range bPaths {
		path := p
		_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
			emitOverlapForRow(aName, bName, aIdx, path, row, line, field, threshold, mh, emit)
			return nil
		})
	}
}

func emitOverlapForRow(aName, bName string, aIdx trainIndex, path string, row int, line []byte, field string, threshold float64, mh *dedup.MinHash, emit func(diag.Finding)) {
	prompt := extractPrompt(line, field)
	if prompt == "" {
		return
	}
	if loc, hit := aIdx.exact[prompt]; hit {
		emit(crossDatasetExactFinding(aName, bName, path, row, loc))
		return
	}
	if threshold > 0 && mh != nil {
		if f, sim, hit := bestFuzzyMatch(prompt, mh, &aIdx, threshold); hit {
			emit(crossDatasetNearDupFinding(aName, bName, path, row, f.loc, sim))
		}
	}
}

func crossDatasetExactFinding(aName, bName, path string, row int, loc promptLoc) diag.Finding {
	return diag.Finding{
		RuleID:   "cross-dataset-overlap",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"prompt also appears in dataset %q at %s row %d (this row is in dataset %q)",
			aName, loc.path, loc.row, bName),
		Location: diag.Location{Path: path, Row: row},
	}
}

func crossDatasetNearDupFinding(aName, bName, path string, row int, loc promptLoc, sim float64) diag.Finding {
	return diag.Finding{
		RuleID:   "cross-dataset-overlap",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"prompt is a near-duplicate (similarity %.2f) of dataset %q at %s row %d (this row is in dataset %q)",
			sim, aName, loc.path, loc.row, bName),
		Location: diag.Location{Path: path, Row: row},
	}
}
