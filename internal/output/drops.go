package output

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
)

// WriteDrops emits one tab-separated line per (path, row) with at
// least one finding whose Row pointer is set: `path\trow\trules`.
// Rules are joined by comma in alphabetical order. Output is sorted
// by (path, row) so the result is deterministic across runs and
// pipes cleanly into awk/xargs/sort.
//
// Findings without a Row (Python AST rules cite Line, not Row) are
// excluded — they don't suggest dropping a data row. Use the json
// or sarif formats for the complete view.
func WriteDrops(w io.Writer, findings []diag.Finding) error {
	groups := groupDataDrops(findings)
	keys := make([]dropKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].row < keys[j].row
	})

	bw := bufio.NewWriter(w)
	for _, k := range keys {
		rules := sortedSet(groups[k])
		if _, err := fmt.Fprintf(bw, "%s\t%d\t%s\n", k.path, k.row, strings.Join(rules, ",")); err != nil {
			return err
		}
	}
	return bw.Flush()
}

type dropKey struct {
	path string
	row  int
}

// groupDataDrops collects every Row-anchored finding under its
// (path, row) key, building a set of rule IDs per row.
func groupDataDrops(findings []diag.Finding) map[dropKey]map[string]struct{} {
	groups := map[dropKey]map[string]struct{}{}
	for _, f := range findings {
		if f.Location.Row <= 0 || f.Location.Path == "" {
			continue
		}
		k := dropKey{path: f.Location.Path, row: f.Location.Row}
		if groups[k] == nil {
			groups[k] = map[string]struct{}{}
		}
		groups[k][f.RuleID] = struct{}{}
	}
	return groups
}

func sortedSet(s map[string]struct{}) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
