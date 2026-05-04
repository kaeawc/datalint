package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/fixer"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/rules"

	// Side-effect import: register the built-in rule set.
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

// toolDescriptor is the shape MCP wants in tools/list. The
// inputSchema is a JSON Schema fragment the client uses to validate
// arguments and present a UI.
type toolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

const lintInputSchema = `{
  "type": "object",
  "properties": {
    "paths": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Per-file paths to lint (.jsonl, .py, .parquet)"
    },
    "train": {
      "type": "array",
      "items": {"type": "string"},
      "description": "JSONL paths in the train split (paired with eval for corpus rules)"
    },
    "eval": {
      "type": "array",
      "items": {"type": "string"},
      "description": "JSONL paths in the eval split (paired with train for corpus rules)"
    }
  }
}`

func (s *Server) respondToolsList(m *Message) error {
	tools := []toolDescriptor{
		{
			Name:        "lint",
			Description: "Run datalint over the supplied per-file paths and/or train/eval splits and return findings.",
			InputSchema: json.RawMessage(lintInputSchema),
		},
		{
			Name:        "fix",
			Description: "Run datalint and apply auto-fixes for findings whose rule emits one (e.g. random-seed-not-set). Modifies files in place. Returns a summary plus the pre-fix findings list.",
			InputSchema: json.RawMessage(lintInputSchema),
		},
	}
	body, err := json.Marshal(map[string]any{"tools": tools})
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

// toolCallParams is the params shape for tools/call.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// lintArgs is the schema-validated argument set for the lint tool.
type lintArgs struct {
	Paths []string `json:"paths"`
	Train []string `json:"train"`
	Eval  []string `json:"eval"`
}

// toolCallResult is the MCP-spec shape for a tool result.
type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) respondToolsCall(m *Message) error {
	var p toolCallParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return s.respond(m, nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()})
	}
	args, err := decodeLintArgs(p.Arguments)
	if err != nil {
		return s.respondToolError(m, "invalid arguments: "+err.Error())
	}
	switch p.Name {
	case "lint":
		return s.respondToolLint(m, args)
	case "fix":
		return s.respondToolFix(m, args)
	}
	return s.respondToolError(m, fmt.Sprintf("unknown tool: %s", p.Name))
}

func decodeLintArgs(raw json.RawMessage) (lintArgs, error) {
	var args lintArgs
	if len(raw) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return args, err
	}
	return args, nil
}

func (s *Server) respondToolLint(m *Message, args lintArgs) error {
	findings := s.runLint(args)
	body, err := buildLintResult(findings)
	if err != nil {
		return s.respondToolError(m, err.Error())
	}
	return s.respond(m, body, nil)
}

func (s *Server) respondToolFix(m *Message, args lintArgs) error {
	findings := s.runLint(args)
	res, err := fixer.Apply(findings)
	if err != nil {
		return s.respondToolError(m, err.Error())
	}
	body, err := buildFixResult(findings, res)
	if err != nil {
		return s.respondToolError(m, err.Error())
	}
	return s.respond(m, body, nil)
}

// buildFixResult emits a single text block: a one-line summary on
// top, then the pretty-printed pre-fix findings so the caller knows
// exactly what was repaired and what's left untouched.
func buildFixResult(findings []diag.Finding, res fixer.Result) (json.RawMessage, error) {
	if findings == nil {
		findings = []diag.Finding{}
	}
	body, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf("Applied %d edit(s) across %d file(s).\n\nFindings (pre-fix):\n%s",
		res.EditsApplied, res.FilesModified, string(body))
	return json.Marshal(toolCallResult{
		Content: []toolContent{{Type: "text", Text: text}},
		IsError: false,
	})
}

func (s *Server) runLint(args lintArgs) []diag.Finding {
	var all []diag.Finding
	if len(args.Paths) > 0 {
		f, err := pipeline.Run(args.Paths, s.cfg)
		if err == nil {
			all = append(all, f...)
		}
	}
	if len(args.Train) > 0 || len(args.Eval) > 0 {
		ctx := &rules.CorpusContext{Train: args.Train, Eval: args.Eval}
		all = append(all, pipeline.RunCorpus(ctx, s.cfg)...)
	}
	return all
}

// buildLintResult formats the findings as a single text content
// block. Pretty-printed JSON keeps it both human-readable in MCP
// inspectors and machine-readable to downstream agents that want to
// re-parse the array.
func buildLintResult(findings []diag.Finding) (json.RawMessage, error) {
	if findings == nil {
		findings = []diag.Finding{}
	}
	body, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return nil, err
	}
	return json.Marshal(toolCallResult{
		Content: []toolContent{{Type: "text", Text: string(body)}},
		IsError: false,
	})
}

// respondToolError returns a successful JSON-RPC response whose
// payload signals tool-level error via isError=true. MCP keeps the
// JSON-RPC error channel for *protocol* errors (bad params, unknown
// method); tool-level failures live inside the result.
func (s *Server) respondToolError(req *Message, msg string) error {
	body, err := json.Marshal(toolCallResult{
		Content: []toolContent{{Type: "text", Text: msg}},
		IsError: true,
	})
	if err != nil {
		return err
	}
	return s.respond(req, body, nil)
}
