package builtin

import (
	"fmt"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
	sitter "github.com/smacker/go-tree-sitter"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "random-seed-not-set",
		Category:   rules.CategoryPipeline,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixIdiomatic,
		Needs:      rules.NeedsPythonAST,
		Check:      checkRandomSeedNotSet,
	})
}

// unseededRNGCalls names dotted call paths whose results depend on
// the current RNG state. v0 only matches the canonical patterns —
// `from random import shuffle` style imports are out of scope until
// the scanner tracks imports.
var unseededRNGCalls = map[string]bool{
	"random.shuffle":           true,
	"random.sample":            true,
	"random.choice":            true,
	"random.choices":           true,
	"np.random.shuffle":        true,
	"np.random.choice":         true,
	"np.random.permutation":    true,
	"np.random.rand":           true,
	"np.random.randint":        true,
	"numpy.random.shuffle":     true,
	"numpy.random.choice":      true,
	"numpy.random.permutation": true,
	"numpy.random.rand":        true,
	"numpy.random.randint":     true,
}

// seedCalls clear the rule for the whole file when present anywhere.
// File-level (not function-level) scope is intentionally permissive
// for v0; refining to scope-aware detection is tracked separately.
var seedCalls = map[string]bool{
	"random.seed":       true,
	"np.random.seed":    true,
	"numpy.random.seed": true,
}

func checkRandomSeedNotSet(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.Python == nil {
		return
	}
	py := ctx.Python

	var unseededHits []*sitter.Node
	seeded := false

	walkPython(py.Tree.RootNode(), func(n *sitter.Node) {
		if n.Type() != "call" {
			return
		}
		fnNode := n.ChildByFieldName("function")
		if fnNode == nil {
			return
		}
		path := dottedPath(fnNode, py.Source)
		if path == "" {
			return
		}
		if seedCalls[path] {
			seeded = true
			return
		}
		if unseededRNGCalls[path] {
			unseededHits = append(unseededHits, n)
		}
	})

	if seeded {
		return
	}
	fix := buildRandomSeedFix(py, unseededHits)
	for _, n := range unseededHits {
		path := dottedPath(n.ChildByFieldName("function"), py.Source)
		emit(diag.Finding{
			RuleID:   "random-seed-not-set",
			Severity: diag.SeverityWarning,
			Message: fmt.Sprintf(
				"%s called without random.seed/np.random.seed earlier in this file; pipeline output is non-reproducible",
				path),
			Location: diag.Location{
				Path: py.Path,
				Line: int(n.StartPoint().Row) + 1,
				Col:  int(n.StartPoint().Column) + 1,
			},
			Fix: fix,
		})
	}
}

// buildRandomSeedFix picks an insertion line and the right seed call
// based on which RNG library the file uses. Inserts after the last
// import statement so the seed call references already-imported
// modules; falls back to line 1 when there are no imports.
func buildRandomSeedFix(py *scanner.PythonFile, unseeded []*sitter.Node) *diag.Fix {
	insertLine := lastImportEndLine(py.Tree.RootNode()) + 1
	if insertLine <= 0 {
		insertLine = 1
	}
	seedCall := pickSeedCall(unseeded, py.Source)
	return &diag.Fix{
		Description: fmt.Sprintf("insert %s at line %d for reproducibility", seedCall, insertLine),
		Level:       diag.FixIdiomatic,
		Edits: []diag.FixEdit{
			{Line: insertLine, Insert: seedCall + "\n"},
		},
	}
}

// pickSeedCall returns the seed invocation that matches the
// namespace of the first unseeded RNG call we found. random.seed for
// random.*, np.random.seed for np.random.*, etc.
func pickSeedCall(unseeded []*sitter.Node, src []byte) string {
	for _, n := range unseeded {
		path := dottedPath(n.ChildByFieldName("function"), src)
		switch {
		case strings.HasPrefix(path, "np.random."):
			return "np.random.seed(0)"
		case strings.HasPrefix(path, "numpy.random."):
			return "numpy.random.seed(0)"
		case strings.HasPrefix(path, "random."):
			return "random.seed(0)"
		}
	}
	return "random.seed(0)"
}

// lastImportEndLine returns the 1-based line number of the last
// import_statement / import_from_statement, or 0 when the file has
// no imports.
func lastImportEndLine(root *sitter.Node) int {
	last := 0
	walkPython(root, func(n *sitter.Node) {
		t := n.Type()
		if t != "import_statement" && t != "import_from_statement" {
			return
		}
		end := int(n.EndPoint().Row) + 1
		if end > last {
			last = end
		}
	})
	return last
}
