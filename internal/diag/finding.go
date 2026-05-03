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

// Location points at the offending position. Line is for source files,
// Row is for streamed data files (JSONL row index, Parquet row group).
type Location struct {
	Path string
	Line int
	Row  int
	Col  int
}

// Finding is one diagnostic from one rule.
type Finding struct {
	RuleID   string
	Severity Severity
	Message  string
	Location Location
}
