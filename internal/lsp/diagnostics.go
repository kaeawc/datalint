package lsp

import (
	"encoding/json"

	"github.com/kaeawc/datalint/internal/diag"
)

// LSP severity levels: 1=Error, 2=Warning, 3=Information, 4=Hint.
const (
	lspError   = 1
	lspWarning = 2
	lspInfo    = 3
)

// publishDiagnosticsParams is the LSP shape sent via the
// textDocument/publishDiagnostics notification.
type publishDiagnosticsParams struct {
	URI         string          `json:"uri"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func (s *Server) publishDiagnostics(uri string, diags []lspDiagnostic) error {
	if diags == nil {
		// LSP wants an empty array (not null) to clear diagnostics.
		diags = []lspDiagnostic{}
	}
	body, err := json.Marshal(publishDiagnosticsParams{URI: uri, Diagnostics: diags})
	if err != nil {
		return err
	}
	return WriteMessage(s.out, &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  body,
	})
}

// toLSPDiagnostics maps Findings to LSP diagnostics, dropping any
// finding whose Location.Path doesn't match the requested file (a
// corpus rule could in theory cite another path; this defensive
// filter keeps the publish per-document).
func toLSPDiagnostics(findings []diag.Finding, path string) []lspDiagnostic {
	out := make([]lspDiagnostic, 0, len(findings))
	for _, f := range findings {
		if f.Location.Path != path {
			continue
		}
		out = append(out, toLSPDiagnostic(f))
	}
	return out
}

func toLSPDiagnostic(f diag.Finding) lspDiagnostic {
	line := f.Location.Line
	if line == 0 && f.Location.Row != 0 {
		line = f.Location.Row
	}
	// LSP positions are 0-based; our Lines/Rows are 1-based.
	zeroLine := line - 1
	if zeroLine < 0 {
		zeroLine = 0
	}
	col := f.Location.Col - 1
	if col < 0 {
		col = 0
	}
	return lspDiagnostic{
		Range: lspRange{
			Start: lspPosition{Line: zeroLine, Character: col},
			End:   lspPosition{Line: zeroLine, Character: col + 1},
		},
		Severity: lspSeverity(f.Severity),
		Code:     f.RuleID,
		Source:   "datalint",
		Message:  f.Message,
	}
}

func lspSeverity(s diag.Severity) int {
	switch s {
	case diag.SeverityError:
		return lspError
	case diag.SeverityWarning:
		return lspWarning
	case diag.SeverityInfo:
		return lspInfo
	}
	return lspInfo
}
