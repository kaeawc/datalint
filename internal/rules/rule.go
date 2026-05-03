// Package rules defines the rule registry and the metadata each rule
// declares so the dispatcher can route work efficiently.
package rules

import (
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
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

// Context is the per-run state passed to each rule's Check function.
// The skeleton keeps it empty; the dispatcher fills it in once scanner
// and indexes exist.
type Context struct{}

// CheckFunc is the rule body. It emits Findings via the provided callback.
type CheckFunc func(ctx *Context, emit func(diag.Finding))

// Rule is the unit of registration.
type Rule struct {
	ID         string
	Category   Category
	Severity   diag.Severity
	Confidence Confidence
	Fix        FixLevel
	Needs      Capability
	Check      CheckFunc
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
