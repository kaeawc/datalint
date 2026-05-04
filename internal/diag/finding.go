// Package diag defines the Finding and Location types emitted by rules
// and consumed by output formatters.
package diag

// Severity classifies how serious a Finding is.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// String returns the canonical lowercase name. Output formatters
// that need a different mapping (SARIF "note" for info, LSP's
// numeric levels) keep their own helpers.
func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	return "info"
}

// Location points at the offending position. Line is for source files,
// Row is for streamed data files (JSONL row index, Parquet row group).
type Location struct {
	Path string
	Line int
	Row  int
	Col  int
}

// FixLevel describes how aggressive a Fix is. The fixer applies all
// levels indiscriminately; consumers (humans reading SARIF, the CLI
// telling the user what changed) use the level to decide trust.
type FixLevel string

const (
	FixCosmetic  FixLevel = "cosmetic"
	FixIdiomatic FixLevel = "idiomatic"
	FixSemantic  FixLevel = "semantic"
)

// FixEdit is one localized text insertion. v0 supports pure insertion
// before a 1-based line number; replacements and deletions are
// follow-ups. Line=0 means "top of file".
type FixEdit struct {
	Line   int
	Insert string
}

// Fix is the optional repair attached to a Finding. Description is
// the human-readable summary the CLI prints when --fix runs.
type Fix struct {
	Description string
	Level       FixLevel
	Edits       []FixEdit
}

// Finding is one diagnostic from one rule. Fix is non-nil when the
// rule offers an automatic repair.
type Finding struct {
	RuleID   string
	Severity Severity
	Message  string
	Location Location
	Fix      *Fix `json:",omitempty"`
}
