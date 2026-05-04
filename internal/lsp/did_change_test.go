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

// TestDidChange_IncrementalEditAppliesAgainstOpenedBuffer verifies
// that a Range-bearing change splices into the in-memory buffer
// rather than replacing it. The opened buffer is clean; the
// incremental change inserts a `random.shuffle(data)` call on a new
// line at the end. The post-change lint must flag the new shuffle.
func TestDidChange_IncrementalEditAppliesAgainstOpenedBuffer(t *testing.T) {
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

	// Opened buffer: "import random\n" — 14 bytes, line 0 ends after
	// "random", line 1 is empty (EOF).
	openParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": "import random\n"},
	})
	// Incremental change: insert "random.shuffle(data)\n" at line 1
	// char 0 (i.e., end of buffer). Range start == end → pure insertion.
	changeParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{
			"range": map[string]any{
				"start": map[string]any{"line": 1, "character": 0},
				"end":   map[string]any{"line": 1, "character": 0},
			},
			"text": "random.shuffle(data)\n",
		}},
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
		t.Fatalf("expected 2 publishes (open clean + incremental change buggy), got %d", len(pubs))
	}
	if len(pubs[0].Diagnostics) != 0 {
		t.Errorf("first publish should be clean, got %d", len(pubs[0].Diagnostics))
	}
	if len(pubs[1].Diagnostics) != 1 {
		t.Fatalf("second publish should flag the inserted shuffle, got %d", len(pubs[1].Diagnostics))
	}
	if !strings.Contains(pubs[1].Diagnostics[0].Message, "random.shuffle") {
		t.Errorf("message should cite the shuffle: %q", pubs[1].Diagnostics[0].Message)
	}
}

// TestDidChange_MixedFullAndIncrementalAppliedInOrder verifies the
// LSP requirement that multiple contentChanges are applied
// sequentially. Sequence: full-replace to a clean buffer, then
// incremental insert of a shuffle call. The final lint must reflect
// the post-incremental buffer, not the post-full-replace one.
func TestDidChange_MixedFullAndIncrementalAppliedInOrder(t *testing.T) {
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
		"textDocument": map[string]any{"uri": uri, "text": "x = 1\n"},
	})
	// Two changes in one notification: (1) full replace with clean
	// "import random\n"; (2) incremental insertion of a shuffle call
	// at the end.
	changeParams, _ := json.Marshal(map[string]any{
		"textDocument": map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{
			{"text": "import random\n"},
			{
				"range": map[string]any{
					"start": map[string]any{"line": 1, "character": 0},
					"end":   map[string]any{"line": 1, "character": 0},
				},
				"text": "random.shuffle(data)\n",
			},
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
		t.Fatalf("expected 2 publishes, got %d", len(pubs))
	}
	if len(pubs[1].Diagnostics) != 1 {
		t.Fatalf("after mixed changes, expected 1 diagnostic, got %d", len(pubs[1].Diagnostics))
	}
	if !strings.Contains(pubs[1].Diagnostics[0].Message, "random.shuffle") {
		t.Errorf("expected shuffle diagnostic, got %q", pubs[1].Diagnostics[0].Message)
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
