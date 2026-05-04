package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaeawc/datalint/internal/dedup"
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

// fuzzyEntry pairs a train-row's MinHash signature with its location
// for the near-duplicate scan path.
type fuzzyEntry struct {
	sig []uint64
	loc promptLoc
	raw string
}

// trainIndex packages the exact-match map and the optional fuzzy
// entries (with LSH bucketing) together so streamEvalAgainstTrain
// has one input.
type trainIndex struct {
	exact map[string]promptLoc
	fuzzy []fuzzyEntry
	lsh   *dedup.LSH
}

// promptFieldDefault is the JSON field whose trimmed string value
// counts as a row's prompt. Override per project via:
//
//	rules:
//	  train-eval-overlap:
//	    prompt_field: input
//	    near_dup_threshold: 0.85
const promptFieldDefault = "prompt"

// checkTrainEvalOverlap streams each train file once to build a map of
// trimmed prompt → first (path, row), then streams each eval file
// and emits one finding per row whose prompt is an exact (or, when
// near_dup_threshold > 0, near-duplicate) match against the train
// index.
//
// near_dup_threshold = 0 (default) keeps the original exact-match
// behavior. With threshold > 0, MinHash signatures (128 hashes,
// 3-token shingles) are computed for every train prompt and each
// eval prompt; the train signatures are also indexed in an LSH
// (32 bands × 4 rows) so each eval row only verifies against
// candidates that share at least one band-bucket — sublinear in
// train size for typical thresholds.
func checkTrainEvalOverlap(ctx *rules.CorpusContext, emit func(diag.Finding)) {
	if ctx == nil || len(ctx.Train) == 0 || len(ctx.Eval) == 0 {
		return
	}
	field := ctx.Settings.String("prompt_field", promptFieldDefault)
	threshold := ctx.Settings.Float("near_dup_threshold", 0.0)

	var mh *dedup.MinHash
	if threshold > 0 {
		mh = dedup.New(0)
	}

	idx := buildTrainIndex(ctx.Train, field, mh)
	if len(idx.exact) == 0 && len(idx.fuzzy) == 0 {
		return
	}
	for _, p := range ctx.Eval {
		streamEvalAgainstTrain(p, field, &idx, threshold, mh, emit)
	}
}

func streamEvalAgainstTrain(p, field string, idx *trainIndex, threshold float64, mh *dedup.MinHash, emit func(diag.Finding)) {
	_ = scanner.StreamJSONL(p, func(row int, line []byte) error {
		prompt := extractPrompt(line, field)
		if prompt == "" {
			return nil
		}
		if loc, hit := idx.exact[prompt]; hit {
			emit(exactFinding(p, row, loc))
			return nil
		}
		if threshold > 0 && mh != nil {
			if f, sim, hit := bestFuzzyMatch(prompt, mh, idx, threshold); hit {
				emit(nearDupFinding(p, row, f.loc, sim))
			}
		}
		return nil
	})
}

func bestFuzzyMatch(prompt string, mh *dedup.MinHash, idx *trainIndex, threshold float64) (fuzzyEntry, float64, bool) {
	sig := mh.Signature(prompt)
	if sig == nil {
		return fuzzyEntry{}, 0, false
	}
	candidates := idx.lsh.Candidates(sig)
	bestIdx := -1
	bestSim := 0.0
	for _, i := range candidates {
		sim := dedup.Similarity(sig, idx.fuzzy[i].sig)
		if sim > bestSim {
			bestSim = sim
			bestIdx = i
		}
	}
	if bestIdx < 0 || bestSim < threshold {
		return fuzzyEntry{}, 0, false
	}
	return idx.fuzzy[bestIdx], bestSim, true
}

func exactFinding(path string, row int, loc promptLoc) diag.Finding {
	return diag.Finding{
		RuleID:   "train-eval-overlap",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"eval prompt also appears in train at %s row %d",
			loc.path, loc.row),
		Location: diag.Location{Path: path, Row: row},
	}
}

func nearDupFinding(path string, row int, loc promptLoc, sim float64) diag.Finding {
	return diag.Finding{
		RuleID:   "train-eval-overlap",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"eval prompt is a near-duplicate (similarity %.2f) of train at %s row %d",
			sim, loc.path, loc.row),
		Location: diag.Location{Path: path, Row: row},
	}
}

func buildTrainIndex(paths []string, field string, mh *dedup.MinHash) trainIndex {
	idx := trainIndex{exact: map[string]promptLoc{}}
	if mh != nil {
		idx.lsh = dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	}
	for _, p := range paths {
		path := p
		_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
			prompt := extractPrompt(line, field)
			if prompt == "" {
				return nil
			}
			if _, exists := idx.exact[prompt]; !exists {
				idx.exact[prompt] = promptLoc{path: path, row: row}
			}
			if mh != nil {
				if sig := mh.Signature(prompt); sig != nil {
					entryIdx := len(idx.fuzzy)
					idx.fuzzy = append(idx.fuzzy, fuzzyEntry{
						sig: sig,
						loc: promptLoc{path: path, row: row},
						raw: prompt,
					})
					idx.lsh.Add(entryIdx, sig)
				}
			}
			return nil
		})
	}
	return idx
}

// extractPrompt returns the trimmed string value of the named field
// or "" when the row isn't a JSON object, lacks the field, or the
// field isn't a string.
func extractPrompt(line []byte, field string) string {
	var obj map[string]any
	if json.Unmarshal(line, &obj) != nil {
		return ""
	}
	v, ok := obj[field].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}
