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

// publishParams is the subset of textDocument/publishDiagnostics
// params the live-lint tests inspect.
type publishParams struct {
	URI         string `json:"uri"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Severity int    `json:"severity"`
		Message  string `json:"message"`
		Range    struct {
			Start struct {
				Line int `json:"line"`
			} `json:"start"`
		} `json:"range"`
	} `json:"diagnostics"`
}

// readPublishMessages drains every textDocument/publishDiagnostics
// notification from out, in order. Other messages (initialize
// responses, etc.) are skipped.
func readPublishMessages(t *testing.T, out *bytes.Buffer) []publishParams {
	t.Helper()
	r := bufio.NewReader(out)
	var pubs []publishParams
	for {
		m, err := lsp.ReadMessage(r)
		if errors.Is(err, io.EOF) {
			return pubs
		}
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		if m.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var p publishParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			t.Fatalf("decode publish: %v", err)
		}
		pubs = append(pubs, p)
	}
}

func TestDidOpen_PublishesFromInMemoryText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	// On-disk file is CLEAN (has random.seed). The didOpen text we
	// send is BUGGY (no seed). The server must lint the in-memory
	// text, not the disk content.
	if err := os.WriteFile(path, []byte("import random\nrandom.seed(0)\nrandom.shuffle(data)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- lsp.Run(pr, out, config.Default()) }()

	uri := "file://" + path
	idInit := json.RawMessage(`1`)
	openParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"text":       "import random\n\nrandom.shuffle(data)\n", // no seed
			"languageId": "python",
			"version":    1,
		},
	})

	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", ID: &idInit, Method: "initialize", Params: json.RawMessage(`{}`)})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "initialized", Params: json.RawMessage(`{}`)})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: openParams})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = pw.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run: %v", err)
	}

	pubs := readPublishMessages(t, out)
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pubs))
	}
	if len(pubs[0].Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(pubs[0].Diagnostics))
	}
	if pubs[0].Diagnostics[0].Code != "random-seed-not-set" {
		t.Errorf("unexpected diagnostic code: %q", pubs[0].Diagnostics[0].Code)
	}
}

func TestDidChange_RelintsAgainstNewText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	if err := os.WriteFile(path, []byte("import random\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- lsp.Run(pr, out, config.Default()) }()

	uri := "file://" + path
	idInit := json.RawMessage(`1`)

	openParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{
			"uri":  uri,
			"text": "import random\n", // clean — no shuffle
		},
	})
	changeParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{
			{"text": "import random\n\nrandom.shuffle(data)\n"}, // now buggy
		},
	})

	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", ID: &idInit, Method: "initialize", Params: json.RawMessage(`{}`)})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: openParams})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: changeParams})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = pw.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run: %v", err)
	}

	pubs := readPublishMessages(t, out)
	if len(pubs) != 2 {
		t.Fatalf("expected 2 publishes (open clean + change buggy), got %d", len(pubs))
	}
	if len(pubs[0].Diagnostics) != 0 {
		t.Errorf("first publish should be clean, got %d diags", len(pubs[0].Diagnostics))
	}
	if len(pubs[1].Diagnostics) != 1 {
		t.Fatalf("second publish should flag the new shuffle, got %d diags", len(pubs[1].Diagnostics))
	}
	if !strings.Contains(pubs[1].Diagnostics[0].Message, "random.shuffle") {
		t.Errorf("message should cite the shuffle: %q", pubs[1].Diagnostics[0].Message)
	}
}

func TestDidClose_ClearsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- lsp.Run(pr, out, config.Default()) }()

	uri := "file://" + path
	idInit := json.RawMessage(`1`)
	openParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{
			"uri":  uri,
			"text": "import random\n\nrandom.shuffle(data)\n",
		},
	})
	closeParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})

	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", ID: &idInit, Method: "initialize", Params: json.RawMessage(`{}`)})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: openParams})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "textDocument/didClose", Params: closeParams})
	mustWrite(t, pw, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = pw.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run: %v", err)
	}

	pubs := readPublishMessages(t, out)
	if len(pubs) != 2 {
		t.Fatalf("expected 2 publishes (open + close-clears), got %d", len(pubs))
	}
	if len(pubs[1].Diagnostics) != 0 {
		t.Errorf("close publish should clear; got %d diags", len(pubs[1].Diagnostics))
	}
}

func mustWrite(t *testing.T, w io.Writer, m *lsp.Message) {
	t.Helper()
	if err := lsp.WriteMessage(w, m); err != nil {
		t.Fatal(err)
	}
}
