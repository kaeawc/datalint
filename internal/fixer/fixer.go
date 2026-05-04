// Package fixer applies the FixEdits attached to findings, in place,
// to the source files they reference.
package fixer

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
)

// Result reports what Apply did.
type Result struct {
	FilesModified int
	EditsApplied  int
}

// Apply walks every Finding's Fix.Edits, dedups identical
// (path, line, insert) triples so a rule that fires N times in a
// file doesn't insert the same line N times, and writes the result
// back to disk per file.
//
// Edits are applied in reverse line order so an earlier insertion
// doesn't shift the line numbers of later ones.
func Apply(findings []diag.Finding) (Result, error) {
	byPath, edits := collect(findings)
	res := Result{}
	paths := sortedKeys(byPath)
	for _, path := range paths {
		if err := applyToFile(path, byPath[path]); err != nil {
			return res, fmt.Errorf("fixer: %s: %w", path, err)
		}
		res.FilesModified++
		res.EditsApplied += len(byPath[path])
	}
	_ = edits // edits is the dedup'd count; Result.EditsApplied carries it
	return res, nil
}

type editKey struct {
	path   string
	line   int
	insert string
}

func collect(findings []diag.Finding) (map[string][]diag.FixEdit, int) {
	byPath := map[string][]diag.FixEdit{}
	seen := map[editKey]bool{}
	count := 0
	for _, f := range findings {
		if f.Fix == nil {
			continue
		}
		for _, e := range f.Fix.Edits {
			k := editKey{f.Location.Path, e.Line, e.Insert}
			if seen[k] {
				continue
			}
			seen[k] = true
			byPath[f.Location.Path] = append(byPath[f.Location.Path], e)
			count++
		}
	}
	return byPath, count
}

func sortedKeys(m map[string][]diag.FixEdit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func applyToFile(path string, edits []diag.FixEdit) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Preserve a trailing newline if the file had one. We split on
	// "\n" and rejoin, so a file ending with "\n" yields a final
	// empty element we put back the same way.
	lines := strings.Split(string(content), "\n")

	// Reverse-line order so earlier inserts don't shift later targets.
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Line > edits[j].Line
	})
	for _, e := range edits {
		idx := e.Line - 1
		if idx < 0 {
			idx = 0
		}
		if idx > len(lines) {
			idx = len(lines)
		}
		insertLines := strings.Split(strings.TrimRight(e.Insert, "\n"), "\n")
		out := make([]string, 0, len(lines)+len(insertLines))
		out = append(out, lines[:idx]...)
		out = append(out, insertLines...)
		out = append(out, lines[idx:]...)
		lines = out
	}
	// path comes from a Finding.Location.Path, which the user passed
	// via CLI args; this is the same trust model as os.Open earlier
	// in the pipeline.
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600) //nolint:gosec // G703: caller-supplied path by design

}
