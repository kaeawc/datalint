package lsp

import (
	"encoding/json"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
)

// codeActionParams is the LSP shape sent on textDocument/codeAction.
// We read just enough to find the document and the cursor range.
type codeActionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Range lspRange `json:"range"`
}

// codeAction is the LSP CodeAction shape we emit. v0 always uses
// kind="quickfix" and a workspace edit with one or more inserts.
type codeAction struct {
	Title       string        `json:"title"`
	Kind        string        `json:"kind"`
	Edit        workspaceEdit `json:"edit"`
	IsPreferred bool          `json:"isPreferred,omitempty"`
}

type workspaceEdit struct {
	Changes map[string][]textEdit `json:"changes"`
}

type textEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

func (s *Server) respondCodeAction(m *Message) error {
	var p codeActionParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return s.respond(m, json.RawMessage("[]"), nil)
	}
	path := uriToPath(p.TextDocument.URI)
	if path == "" {
		return s.respond(m, json.RawMessage("[]"), nil)
	}

	findings, err := pipeline.Run([]string{path}, s.cfg)
	if err != nil {
		return s.respond(m, json.RawMessage("[]"), nil)
	}

	actions := buildCodeActions(p.TextDocument.URI, p.Range, findings)
	body, err := json.Marshal(actions)
	if err != nil {
		return s.respond(m, json.RawMessage("[]"), nil)
	}
	return s.respond(m, body, nil)
}

func buildCodeActions(uri string, selection lspRange, findings []diag.Finding) []codeAction {
	out := []codeAction{}
	for _, f := range findings {
		if f.Fix == nil {
			continue
		}
		if !findingInSelection(f, selection) {
			continue
		}
		out = append(out, fixToCodeAction(uri, f))
	}
	return out
}

// findingInSelection reports whether a finding's anchor line falls
// within the editor's requested range. LSP positions are 0-based;
// our Line / Row are 1-based, so we convert before comparing. A
// finding with no Line/Row defaults to line 0, which only matches a
// selection starting at the file head — defensible for unanchored
// findings (rare).
func findingInSelection(f diag.Finding, sel lspRange) bool {
	line := f.Location.Line
	if line == 0 && f.Location.Row != 0 {
		line = f.Location.Row
	}
	zeroLine := line - 1
	if zeroLine < 0 {
		zeroLine = 0
	}
	return zeroLine >= sel.Start.Line && zeroLine <= sel.End.Line
}

// fixToCodeAction packs a Fix into one CodeAction with all its edits
// expressed as zero-width inserts. Each FixEdit at 1-based line N
// becomes an LSP edit at 0-based line N-1, character 0.
func fixToCodeAction(uri string, f diag.Finding) codeAction {
	edits := make([]textEdit, 0, len(f.Fix.Edits))
	for _, e := range f.Fix.Edits {
		zeroLine := e.Line - 1
		if zeroLine < 0 {
			zeroLine = 0
		}
		edits = append(edits, textEdit{
			Range: lspRange{
				Start: lspPosition{Line: zeroLine, Character: 0},
				End:   lspPosition{Line: zeroLine, Character: 0},
			},
			NewText: e.Insert,
		})
	}
	title := f.Fix.Description
	if title == "" {
		title = f.RuleID
	}
	return codeAction{
		Title:       title,
		Kind:        "quickfix",
		Edit:        workspaceEdit{Changes: map[string][]textEdit{uri: edits}},
		IsPreferred: true,
	}
}
