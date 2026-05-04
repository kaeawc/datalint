package builtin

import (
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	sitter "github.com/smacker/go-tree-sitter"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "dedup-key-misses-normalization",
		Category:   rules.CategoryPipeline,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceLow,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsPythonAST,
		Check:      checkDedupNormalization,
	})
}

// dedupCallNames are callee names that look like deduplication
// operations on text. Builtins like `set` are inherently fuzzy —
// `set([1, 2, 3])` dedups numbers and doesn't need text normalization
// — so the rule's confidence is intentionally low.
var dedupCallNames = map[string]bool{
	"drop_duplicates": true, // pandas DataFrame.drop_duplicates
	"unique":          true, // np.unique, pd.unique, Series.unique
	"set":             true, // builtin set() used as dedup
}

// normalizationCallNames are callee names whose presence anywhere in
// the file we treat as evidence the pipeline already normalizes.
// Conservative: only count text-y normalizations.
var normalizationCallNames = map[string]bool{
	"lower":     true,
	"upper":     true,
	"casefold":  true,
	"strip":     true,
	"lstrip":    true,
	"rstrip":    true,
	"normalize": true, // unicodedata.normalize
}

// checkDedupNormalization flags Python files that call a dedup-shaped
// helper but never call any normalization-shaped helper anywhere in
// the same file. v0 is a presence/absence check at file scope —
// function-scope and dataflow analysis are explicit follow-ups.
func checkDedupNormalization(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.Python == nil {
		return
	}
	py := ctx.Python

	var firstDedup *sitter.Node
	var firstDedupName string
	sawNorm := false

	walkPython(py.Tree.RootNode(), func(n *sitter.Node) {
		if n.Type() != "call" {
			return
		}
		name := calleeBaseName(n.ChildByFieldName("function"), py.Source)
		if name == "" {
			return
		}
		if normalizationCallNames[name] {
			sawNorm = true
			return
		}
		if dedupCallNames[name] && firstDedup == nil {
			firstDedup = n
			firstDedupName = name
		}
	})

	if firstDedup == nil || sawNorm {
		return
	}
	emit(diag.Finding{
		RuleID:   "dedup-key-misses-normalization",
		Severity: diag.SeverityWarning,
		Message: fmt.Sprintf(
			"%s called with no string normalization (lower/strip/casefold/unicodedata.normalize) elsewhere in this file; near-duplicates may slip through",
			firstDedupName),
		Location: diag.Location{
			Path: py.Path,
			Line: int(firstDedup.StartPoint().Row) + 1,
			Col:  int(firstDedup.StartPoint().Column) + 1,
		},
	})
}
