package builtin

import (
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
	sitter "github.com/smacker/go-tree-sitter"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "shuffle-after-split",
		Category:   rules.CategoryPipeline,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsPythonAST,
		Check:      checkShuffleAfterSplit,
	})
}

// splitCalls covers the dotted paths that perform a train/eval split
// of a dataset. v0 sticks to sklearn's canonical helper; manual slice
// splits are tracked as a follow-up.
var splitCalls = map[string]bool{
	"train_test_split":                         true,
	"sklearn.model_selection.train_test_split": true,
	"model_selection.train_test_split":         true,
}

// shuffleCalls reorder a sequence in place or return a permutation.
// Reusing the same set as random-seed-not-set's unseeded list would
// leak unrelated semantics; this rule only cares about ordering, not
// seeding.
var shuffleCalls = map[string]bool{
	"random.shuffle":           true,
	"np.random.shuffle":        true,
	"np.random.permutation":    true,
	"numpy.random.shuffle":     true,
	"numpy.random.permutation": true,
}

// shuffleCollector is the running state of the AST walk: the first
// split call's byte offset (once seen) and every top-level shuffle
// call collected for a later ordering comparison.
type shuffleCollector struct {
	splitFound     bool
	firstSplitByte uint32
	shuffles       []*sitter.Node
}

// checkShuffleAfterSplit flags shuffle calls that lexically follow
// the first split call in the same file. Calls nested inside another
// call are skipped — `train_test_split(random.shuffle(x))` shows the
// shuffle's source position after the split's, but the shuffle is
// the split's argument and runs first.
func checkShuffleAfterSplit(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.Python == nil {
		return
	}
	py := ctx.Python

	c := &shuffleCollector{}
	walkPython(py.Tree.RootNode(), func(n *sitter.Node) {
		c.visit(n, py.Source)
	})

	if !c.splitFound {
		return
	}
	for _, s := range c.shuffles {
		if s.StartByte() <= c.firstSplitByte {
			continue
		}
		emitShuffleAfterSplit(s, py, emit)
	}
}

func (c *shuffleCollector) visit(n *sitter.Node, src []byte) {
	if n.Type() != "call" {
		return
	}
	fnNode := n.ChildByFieldName("function")
	if fnNode == nil {
		return
	}
	path := dottedPath(fnNode, src)
	if path == "" || isNestedInCall(n) {
		return
	}
	if splitCalls[path] {
		if !c.splitFound {
			c.splitFound = true
			c.firstSplitByte = n.StartByte()
		}
		return
	}
	if shuffleCalls[path] {
		c.shuffles = append(c.shuffles, n)
	}
}

func emitShuffleAfterSplit(s *sitter.Node, py *scanner.PythonFile, emit func(diag.Finding)) {
	path := dottedPath(s.ChildByFieldName("function"), py.Source)
	emit(diag.Finding{
		RuleID:   "shuffle-after-split",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"%s called after train/eval split; either shuffle before splitting or seed each split's RNG explicitly",
			path),
		Location: diag.Location{
			Path: py.Path,
			Line: int(s.StartPoint().Row) + 1,
			Col:  int(s.StartPoint().Column) + 1,
		},
	})
}
