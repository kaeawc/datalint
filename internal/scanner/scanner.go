// Package scanner reads source files (Python pipeline code via tree-sitter)
// and data files (JSONL today; Parquet, MDS, WebDataset later) and exposes
// them to the rule dispatcher.
package scanner

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
)

// Kind classifies a file by how the dispatcher should hand it to rules.
type Kind int

const (
	KindUnknown Kind = iota
	KindPythonSource
	KindJSONL
	KindParquet
)

// File is a parsed source or data file ready for rule dispatch. The
// skeleton carries Path and Kind; AST handles, line indexes, and row
// iterators land here as the scanner is implemented.
type File struct {
	Path string
	Kind Kind
}

// Classify maps a path to a File based on extension. Unknown extensions
// produce KindUnknown.
func Classify(path string) *File {
	kind := KindUnknown
	switch {
	case strings.HasSuffix(path, ".jsonl"):
		kind = KindJSONL
	case strings.HasSuffix(path, ".py"):
		kind = KindPythonSource
	case strings.HasSuffix(path, ".parquet"):
		kind = KindParquet
	}
	return &File{Path: path, Kind: kind}
}

// StreamJSONL invokes fn once per physical line. row is 1-based; line
// has CR/LF stripped. Blank lines are delivered too — JSONL has no
// notion of a blank record, so callers decide whether to flag them.
//
// The reader handles arbitrarily long lines (no bufio.Scanner buffer
// limit). If fn returns an error, iteration stops and that error is
// returned.
func StreamJSONL(path string, fn func(row int, line []byte) error) (err error) {
	f, openErr := os.Open(path)
	if openErr != nil {
		return openErr
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	r := bufio.NewReader(f)
	row := 0
	for {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			row++
			line = bytes.TrimRight(line, "\r\n")
			if cbErr := fn(row, line); cbErr != nil {
				return cbErr
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
