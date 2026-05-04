package lsp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/lsp"
)

// codeActionResponse mirrors the LSP CodeAction[] result shape we
// emit. Defined here as the structural contract tests pin.
type codeActionResponse []struct {
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	IsPreferred bool   `json:"isPreferred"`
	Edit        struct {
		Changes map[string][]struct {
			Range struct {
				Start struct {
					Line, Character int
				} `json:"start"`
				End struct {
					Line, Character int
				} `json:"end"`
			} `json:"range"`
			NewText string `json:"newText"`
		} `json:"changes"`
	} `json:"edit"`
}

func sendCodeActionRequest(t *testing.T, path string, startLine, endLine int) codeActionResponse {
	t.Helper()
	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- lsp.Run(pr, out, config.Default()) }()

	id := json.RawMessage(`9`)
	uri := "file://" + path
	params := []byte(`{
		"textDocument": {"uri": "` + uri + `"},
		"range": {
			"start": {"line": ` + itoa(startLine) + `, "character": 0},
			"end":   {"line": ` + itoa(endLine) + `, "character": 0}
		},
		"context": {"diagnostics": []}
	}`)
	if err := lsp.WriteMessage(pw, &lsp.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "textDocument/codeAction",
		Params:  json.RawMessage(params),
	}); err != nil {
		t.Fatal(err)
	}
	if err := lsp.WriteMessage(pw, &lsp.Message{JSONRPC: "2.0", Method: "exit"}); err != nil {
		t.Fatal(err)
	}
	_ = pw.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run returned %v", err)
	}

	resp, err := lsp.ReadMessage(bufio.NewReader(out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var actions codeActionResponse
	if err := json.Unmarshal(resp.Result, &actions); err != nil {
		t.Fatalf("decode result: %v\n%s", err, string(resp.Result))
	}
	return actions
}

func itoa(i int) string {
	switch {
	case i < 0:
		return "-" + itoa(-i)
	case i < 10:
		return string(rune('0' + i))
	}
	return itoa(i/10) + itoa(i%10)
}

func TestCodeAction_FixOnSelectedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	// random.shuffle is on line 3 (0-based 2).
	if err := os.WriteFile(path, []byte("import random\n\nrandom.shuffle(data)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Editor selection covers line 2 (0-based) — the shuffle line.
	actions := sendCodeActionRequest(t, path, 2, 2)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Kind != "quickfix" {
		t.Errorf("kind = %q, want quickfix", a.Kind)
	}
	if !a.IsPreferred {
		t.Errorf("IsPreferred should be true")
	}
	if !strings.Contains(a.Title, "random.seed") {
		t.Errorf("title missing seed call: %q", a.Title)
	}

	uri := "file://" + path
	edits := a.Edit.Changes[uri]
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edits))
	}
	if edits[0].NewText != "random.seed(0)\n" {
		t.Errorf("newText = %q", edits[0].NewText)
	}
	// Fix inserts at 1-based line 2 (after `import random`); LSP edit
	// is at 0-based line 1 with character 0 (zero-width insertion).
	if edits[0].Range.Start.Line != 1 || edits[0].Range.Start.Character != 0 {
		t.Errorf("start = (%d,%d), want (1,0)", edits[0].Range.Start.Line, edits[0].Range.Start.Character)
	}
	if edits[0].Range.End.Line != 1 || edits[0].Range.End.Character != 0 {
		t.Errorf("end = (%d,%d), want (1,0)", edits[0].Range.End.Line, edits[0].Range.End.Character)
	}
}

func TestCodeAction_RangeOutsideFindingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	if err := os.WriteFile(path, []byte("import random\n\nrandom.shuffle(data)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Selection covers only line 0 (the import). The shuffle finding
	// is at line 2 — out of range; no actions.
	actions := sendCodeActionRequest(t, path, 0, 0)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d: %+v", len(actions), actions)
	}
}

func TestCodeAction_FindingWithoutFixIsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")
	// jsonl-malformed-line fires on row 2 but doesn't emit a Fix —
	// so even if the selection matches, no code action.
	body := "{\"id\": 1}\n" + "not json\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	actions := sendCodeActionRequest(t, path, 0, 5)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for fix-less finding, got %d: %+v", len(actions), actions)
	}
}
