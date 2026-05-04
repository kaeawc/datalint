// Package suppression collects per-file rule disables from inline
// comments (Python) and row metadata (JSONL), and tells the dispatcher
// whether a given Finding should be filtered out.
//
// Python source: a `# datalint:disable=<rule-id>` comment anywhere on
// a line suppresses that rule for any finding whose Line matches.
//
// JSONL data: a row-level `_datalint_disable` array of rule IDs
// suppresses those rules for any finding whose Row matches.
//
// Suppression by malformed-line is impossible by construction —
// jsonl-malformed-line fires precisely when the row can't be parsed
// to read its disable field.
package suppression

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/kaeawc/datalint/internal/diag"
)

// Set is the per-file disable map. Both maps default to nil and are
// only allocated when a disable is actually found.
type Set struct {
	byLine map[int]map[string]bool
	byRow  map[int]map[string]bool
}

// Suppresses reports whether f should be filtered out based on this
// set's disables. A Finding is suppressed if either its Line carries
// a comment-disable for the rule (Python) or its Row carries a
// metadata-disable for the rule (JSONL).
func (s Set) Suppresses(f diag.Finding) bool {
	if f.Location.Line > 0 && s.byLine[f.Location.Line][f.RuleID] {
		return true
	}
	if f.Location.Row > 0 && s.byRow[f.Location.Row][f.RuleID] {
		return true
	}
	return false
}

// ExtractFromFile dispatches to the right extractor by extension.
// Errors yield an empty Set so the caller can keep going — bad I/O
// shouldn't make findings disappear; jsonl-malformed-line and
// friends will surface real file issues.
func ExtractFromFile(path string) Set {
	switch {
	case strings.HasSuffix(path, ".py"):
		return ExtractFromPython(path)
	case strings.HasSuffix(path, ".jsonl"):
		return ExtractFromJSONL(path)
	}
	return Set{}
}

// pythonDisableRe captures the rule-id list after `# datalint:disable=`
// on a Python source line. The list is comma-separated (matching
// pylint/ruff convention); whitespace within the list is allowed.
var pythonDisableRe = regexp.MustCompile(`#\s*datalint:disable=([a-zA-Z][a-zA-Z0-9_,\- ]*[a-zA-Z0-9])`)

// ExtractFromPython scans the file line by line, recording every
// `# datalint:disable=<rule-id>[,<rule-id>...]` match.
func ExtractFromPython(path string) Set {
	f, err := os.Open(path)
	if err != nil {
		return Set{}
	}
	defer f.Close()

	s := Set{byLine: map[int]map[string]bool{}}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		for _, m := range pythonDisableRe.FindAllStringSubmatch(sc.Text(), -1) {
			for _, id := range strings.Split(m[1], ",") {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if s.byLine[line] == nil {
					s.byLine[line] = map[string]bool{}
				}
				s.byLine[line][id] = true
			}
		}
	}
	return s
}

// ExtractFromJSONL streams each row, decoding only the
// `_datalint_disable` field, and records the rule IDs for that row.
// Rows that fail to parse are silently skipped — the malformed-line
// rule reports them.
func ExtractFromJSONL(path string) Set {
	f, err := os.Open(path)
	if err != nil {
		return Set{}
	}
	defer f.Close()

	s := Set{byRow: map[int]map[string]bool{}}
	r := bufio.NewReader(f)
	row := 0
	for {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			row++
			recordJSONLDisable(s, row, line)
		}
		if errors.Is(readErr, io.EOF) {
			return s
		}
		if readErr != nil {
			return s
		}
	}
}

func recordJSONLDisable(s Set, row int, line []byte) {
	var obj struct {
		Disable []string `json:"_datalint_disable"`
	}
	if json.Unmarshal(line, &obj) != nil {
		return
	}
	if len(obj.Disable) == 0 {
		return
	}
	if s.byRow[row] == nil {
		s.byRow[row] = map[string]bool{}
	}
	for _, id := range obj.Disable {
		s.byRow[row][id] = true
	}
}

// Filter returns findings minus any suppressed by markers in their
// own files. Sets are cached per path so a single file is parsed
// once even if it produced many findings.
func Filter(findings []diag.Finding) []diag.Finding {
	if len(findings) == 0 {
		return findings
	}
	cache := map[string]Set{}
	out := make([]diag.Finding, 0, len(findings))
	for _, f := range findings {
		s, ok := cache[f.Location.Path]
		if !ok {
			s = ExtractFromFile(f.Location.Path)
			cache[f.Location.Path] = s
		}
		if s.Suppresses(f) {
			continue
		}
		out = append(out, f)
	}
	return out
}
