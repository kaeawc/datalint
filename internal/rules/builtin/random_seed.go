package builtin

import (
	"fmt"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	sitter "github.com/smacker/go-tree-sitter"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "random-seed-not-set",
		Category:   rules.CategoryPipeline,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
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
		})
	}
}

// walkPython visits every descendant node depth-first.
func walkPython(n *sitter.Node, fn func(*sitter.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for i := uint32(0); i < n.ChildCount(); i++ {
		walkPython(n.Child(int(i)), fn)
	}
}

// dottedPath returns the dotted-attribute path of an expression like
// "np.random.shuffle". Returns "" for anything that isn't a simple
// chain of identifiers — indexed/computed/parenthesized expressions
// are intentionally rejected so the rule stays high-precision.
func dottedPath(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	var parts []string
	cur := n
	for cur != nil {
		switch cur.Type() {
		case "attribute":
			attr := cur.ChildByFieldName("attribute")
			if attr == nil {
				return ""
			}
			parts = append([]string{attr.Content(src)}, parts...)
			cur = cur.ChildByFieldName("object")
		case "identifier":
			parts = append([]string{cur.Content(src)}, parts...)
			cur = nil
		default:
			return ""
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}
