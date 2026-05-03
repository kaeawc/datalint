// Package scanner reads source files (Python pipeline code via tree-sitter)
// and data files (JSONL today; Parquet, MDS, WebDataset later) and exposes
// them to the rule dispatcher.
package scanner

// Kind classifies a file by how the dispatcher should hand it to rules.
type Kind int

const (
	KindUnknown Kind = iota
	KindPythonSource
	KindJSONL
	KindParquet
)

// File is a parsed source or data file ready for rule dispatch. The
// skeleton carries only Path and Kind; AST handles, line indexes, and
// row iterators land here as the scanner is implemented.
type File struct {
	Path string
	Kind Kind
}
