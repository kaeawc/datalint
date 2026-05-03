// Package rules defines the rule registry and the metadata each rule
// declares so the dispatcher can route work efficiently.
package rules

import (
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/scanner"
)

// Category groups rules by what they inspect.
type Category string

const (
	CategorySchema       Category = "schema"
	CategoryConversation Category = "conversation"
	CategoryLeakage      Category = "leakage"
	CategoryPipeline     Category = "pipeline"
	CategoryFile         Category = "file"
)

// FixLevel matches Krit's fix-safety tiers.
type FixLevel int

const (
	FixNone FixLevel = iota
	FixCosmetic
	FixIdiomatic
	FixSemantic
)

// Confidence is the rule's self-reported precision tier.
type Confidence int

const (
	ConfidenceLow Confidence = iota
	ConfidenceMedium
	ConfidenceHigh
)

// Capability flags the project context a rule needs. Rules that do not
// declare a capability cannot rely on the dispatcher producing it.
type Capability uint64

const (
	NeedsPythonAST Capability = 1 << iota
	NeedsJSONL
	NeedsParquet
	NeedsCorpusScan
	NeedsLSH
	NeedsExternalEvalSet
)

// Has reports whether c contains every bit in want.
func (c Capability) Has(want Capability) bool { return c&want == want }

// Context is the per-file state passed to each rule's Check function.
// Python is non-nil only when File.Kind == KindPythonSource and the
// dispatcher succeeded in parsing the file.
type Context struct {
	File   *scanner.File
	Python *scanner.PythonFile
}

// CheckFunc is the rule body for per-file rules. It emits Findings via
// the provided callback.
type CheckFunc func(ctx *Context, emit func(diag.Finding))

// CorpusContext carries the cross-file inputs available to corpus-scope
// rules. Train and Eval are the JSONL paths the user grouped under
// `--train` / `--eval`. Either may be empty.
type CorpusContext struct {
	Train []string
	Eval  []string
}

// CorpusCheckFunc runs once per datalint invocation against the full
// corpus. Useful for cross-file rules: leakage, deduplication, and
// anything else that needs to compare rows across files.
type CorpusCheckFunc func(ctx *CorpusContext, emit func(diag.Finding))

// Rule is the unit of registration. A rule provides exactly one of
// Check (per-file) or CorpusCheck (corpus-scope); registering both
// or neither is a programming error.
type Rule struct {
	ID          string
	Category    Category
	Severity    diag.Severity
	Confidence  Confidence
	Fix         FixLevel
	Needs       Capability
	Check       CheckFunc
	CorpusCheck CorpusCheckFunc
}

// IsCorpusScope reports whether this rule expects the corpus dispatch
// path rather than per-file.
func (r *Rule) IsCorpusScope() bool { return r.CorpusCheck != nil }

// AppliesTo reports whether the rule should run against the given file
// based on its declared capabilities and the file's Kind. Corpus-scope
// rules never apply per-file.
func (r *Rule) AppliesTo(f *scanner.File) bool {
	if f == nil || r.IsCorpusScope() {
		return false
	}
	switch f.Kind {
	case scanner.KindJSONL:
		return r.Needs.Has(NeedsJSONL)
	case scanner.KindPythonSource:
		return r.Needs.Has(NeedsPythonAST)
	case scanner.KindParquet:
		return r.Needs.Has(NeedsParquet)
	}
	return false
}

var registry = map[string]*Rule{}

// Register adds a Rule to the global registry. Intended to be called
// from package init().
func Register(r *Rule) {
	if r == nil || r.ID == "" {
		panic("rules.Register: rule must have a non-empty ID")
	}
	if _, exists := registry[r.ID]; exists {
		panic(fmt.Sprintf("rules.Register: duplicate rule ID %q", r.ID))
	}
	if (r.Check == nil) == (r.CorpusCheck == nil) {
		panic(fmt.Sprintf("rules.Register: rule %q must set exactly one of Check or CorpusCheck", r.ID))
	}
	registry[r.ID] = r
}

// All returns every registered Rule in arbitrary order.
func All() []*Rule {
	out := make([]*Rule, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	return out
}

// ByID returns the rule with the given ID, or nil if not registered.
func ByID(id string) *Rule {
	return registry[id]
}
