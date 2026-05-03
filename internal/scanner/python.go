package scanner

import (
	"context"
	"fmt"
	"os"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// PythonFile is a parsed Python source file. Source is the raw byte
// slice the tree references; rules call Tree.RootNode() to walk it.
type PythonFile struct {
	Path   string
	Source []byte
	Tree   *sitter.Tree
}

// ParsePython reads and parses a Python source file with tree-sitter.
func ParsePython(path string) (*PythonFile, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &PythonFile{Path: path, Source: source, Tree: tree}, nil
}
