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

// anchorLater / anchorEarlier are the supported anchor values. The
// rule's `anchor` config selects which side of each pair hosts the
// finding. Anything else falls back to anchorLater silently — bad
// config shouldn't kill the whole run.
const (
	anchorLater   = "later"
	anchorEarlier = "earlier"
)

// checkCrossDatasetOverlap is the N-way generalization of
// train-eval-overlap. For each ordered pair (a, b) of datasets where
// a < b lexically, it builds the train index for a (exact map +
// optional MinHash signatures + LSH) and streams b's rows against
// it, emitting a finding when a prompt matches.
//
// `anchor: later` (default) anchors findings on b — useful for
// leakage workflows where the user typically removes rows from the
// dataset that came in second. `anchor: earlier` swaps the
// direction so findings land on a — useful when the eval set is
// the canonical source and train rows are the ones to scrub.
//
// Either way, every overlap surfaces exactly once.
func checkCrossDatasetOverlap(ctx *rules.CorpusContext, emit func(diag.Finding)) {
	if ctx == nil || len(ctx.Datasets) < 2 {
		return
	}
	field := ctx.Settings.String("prompt_field", promptFieldDefault)
	threshold := ctx.Settings.Float("near_dup_threshold", 0.0)
	anchor := ctx.Settings.String("anchor", anchorLater)

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
			indexed, streamed := pairForAnchor(anchor, a, b)
			emitPairwiseOverlap(indexed, streamed, indices[indexed], ctx.Datasets[streamed], field, threshold, mh, emit)
		}
	}
}

// pairForAnchor returns (indexedName, streamedName). Findings are
// emitted on the streamed side, so:
//
//   - anchor=later (default): index a, stream b → findings on b
//     (the lex-later dataset)
//   - anchor=earlier: index b, stream a → findings on a (the
//     lex-earlier dataset)
//
// Unknown anchor values fall through to the later branch.
func pairForAnchor(anchor, a, b string) (indexed, streamed string) {
	if anchor == anchorEarlier {
		return b, a
	}
	return a, b
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
