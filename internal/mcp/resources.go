package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kaeawc/datalint/internal/rules"
)

// resourceURI* are the stable URIs the server exposes via
// resources/list. Adding a new resource means picking a new URI here
// and wiring it through readBuiltinResource.
const (
	resourceURIRulesIndex    = "datalint:rules/index"
	resourceURIConfigExample = "datalint:config/example"
)

// resourceDescriptor is the MCP shape returned from resources/list.
type resourceDescriptor struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// resourceContent is the per-URI payload returned from resources/read.
// MCP allows multiple content blocks per URI; v0 always returns one.
type resourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

func (s *Server) respondResourcesList(m *Message) error {
	resources := []resourceDescriptor{
		{
			URI:         resourceURIRulesIndex,
			Name:        "datalint rules index",
			Description: "Markdown table of every registered rule with category, severity, confidence, and auto-fix tier.",
			MimeType:    "text/markdown",
		},
		{
			URI:         resourceURIConfigExample,
			Name:        "datalint config example",
			Description: "Annotated datalint.yml showing every config knob the built-in rules read.",
			MimeType:    "text/yaml",
		},
	}
	body, err := json.Marshal(map[string]any{"resources": resources})
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

type resourcesReadParams struct {
	URI string `json:"uri"`
}

func (s *Server) respondResourcesRead(m *Message) error {
	var p resourcesReadParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return s.respond(m, nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()})
	}
	text, mime, ok := readBuiltinResource(p.URI)
	if !ok {
		return s.respond(m, nil, &RPCError{
			Code:    -32601,
			Message: fmt.Sprintf("unknown resource: %q", p.URI),
		})
	}
	body, err := json.Marshal(map[string]any{
		"contents": []resourceContent{{URI: p.URI, MimeType: mime, Text: text}},
	})
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

func readBuiltinResource(uri string) (text, mime string, ok bool) {
	switch uri {
	case resourceURIRulesIndex:
		return buildRulesIndex(), "text/markdown", true
	case resourceURIConfigExample:
		return exampleConfigYAML, "text/yaml", true
	}
	return "", "", false
}

// buildRulesIndex iterates the rule registry and renders a Markdown
// table. Rule order is alphabetical by ID for stable output.
func buildRulesIndex() string {
	registered := rules.All()
	sort.Slice(registered, func(i, j int) bool {
		return registered[i].ID < registered[j].ID
	})

	var b strings.Builder
	b.WriteString("# datalint rules\n\n")
	b.WriteString("| ID | Category | Severity | Confidence | Auto-fix | Scope |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, r := range registered {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s |\n",
			r.ID,
			string(r.Category),
			r.Severity,
			r.Confidence,
			r.Fix,
			scopeLabel(r),
		)
	}
	return b.String()
}

func scopeLabel(r *rules.Rule) string {
	if r.IsCorpusScope() {
		return "corpus"
	}
	return "per-file"
}

// exampleConfigYAML is a checked-in snapshot of every config knob
// the built-in rules read. Update when adding new knobs so agents
// reading this resource see them.
const exampleConfigYAML = `# datalint.yml — every config knob the built-in rules read.

enable: []   # if non-empty, only these rules run (subject to disable below)
disable: []  # rules in this list never run

rules:
  enum-drift:
    lock_in_rows: 5         # default 5; raise for production
    max_distinct: 8         # >max_distinct distinct values in lock-in window → field is free-text, skip

  optional-field-required-by-downstream:
    min_presence_ratio: 0.8
    min_rows: 5
    required_fields:        # explicit schema; overrides ratio for these fields
      - input
      - output

  field-type-mismatch-with-schema:
    field_types:            # field → JSON type (string|number|boolean|array|object|null)
      input: string
      output: string
      score: number
      tags: array

  train-eval-overlap:
    prompt_field: prompt
    near_dup_threshold: 0   # 0 = exact-match only; 0.85 typical for MinHash near-dup

  cross-dataset-overlap:
    prompt_field: prompt
    near_dup_threshold: 0
    anchor: later           # later (default) | earlier — which side of each pair hosts findings

  parquet-row-group-too-large-for-streaming:
    max_rows_per_group: 1000000

  system-prompt-leaks-eval-instructions:
    extra_patterns:
      - "(?i)reply with one of"

  privacy-pii-detected:
    extra_patterns:
      - "internal-id=INT-\\d{6,}"
`
