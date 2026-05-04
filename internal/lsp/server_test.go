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

// scriptedClient is the read side of a fake client: it pre-loads a
// sequence of LSP messages into a Pipe, then Run() consumes them.
type scriptedClient struct {
	in  *io.PipeWriter
	out *bytes.Buffer
}

func newScriptedClient() (*scriptedClient, *io.PipeReader, *bytes.Buffer) {
	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	return &scriptedClient{in: pw, out: out}, pr, out
}

func (c *scriptedClient) send(t *testing.T, m *lsp.Message) {
	t.Helper()
	if err := lsp.WriteMessage(c.in, m); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestServer_InitializeHandshake(t *testing.T) {
	client, serverIn, out := newScriptedClient()
	done := make(chan error, 1)
	go func() {
		done <- lsp.Run(serverIn, out, config.Default())
	}()

	id := json.RawMessage(`1`)
	client.send(t, &lsp.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	})
	client.send(t, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = client.in.Close()

	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run returned %v", err)
	}

	resp := readOnly(t, out)
	if resp.ID == nil || string(*resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
	var result struct {
		Capabilities struct {
			TextDocumentSync   int  `json:"textDocumentSync"`
			DiagnosticProvider bool `json:"diagnosticProvider"`
			CodeActionProvider bool `json:"codeActionProvider"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Capabilities.TextDocumentSync != 2 {
		t.Errorf("textDocumentSync = %d, want 2 (Incremental)", result.Capabilities.TextDocumentSync)
	}
	if !result.Capabilities.DiagnosticProvider {
		t.Errorf("diagnosticProvider should be true")
	}
	if !result.Capabilities.CodeActionProvider {
		t.Errorf("codeActionProvider should be true")
	}
	if result.ServerInfo.Name != "datalint-lsp" {
		t.Errorf("server name = %q, want datalint-lsp", result.ServerInfo.Name)
	}
}

func TestServer_DidSavePublishesDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	body := "import random\n\nrandom.shuffle(data)\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	client, serverIn, out := newScriptedClient()
	done := make(chan error, 1)
	go func() {
		done <- lsp.Run(serverIn, out, config.Default())
	}()

	uri := "file://" + path
	id := json.RawMessage(`42`)
	client.send(t, &lsp.Message{JSONRPC: "2.0", ID: &id, Method: "initialize", Params: json.RawMessage(`{}`)})
	client.send(t, &lsp.Message{JSONRPC: "2.0", Method: "initialized", Params: json.RawMessage(`{}`)})
	client.send(t, &lsp.Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didSave",
		Params:  json.RawMessage(`{"textDocument":{"uri":"` + uri + `"}}`),
	})
	client.send(t, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = client.in.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run returned %v", err)
	}

	// First message: initialize response. Second: publishDiagnostics.
	r := bufio.NewReader(out)
	first, err := lsp.ReadMessage(r)
	if err != nil {
		t.Fatalf("read initialize: %v", err)
	}
	if first.Method != "" {
		t.Fatalf("first message should be a response, got method=%q", first.Method)
	}
	second, err := lsp.ReadMessage(r)
	if err != nil {
		t.Fatalf("read publishDiagnostics: %v", err)
	}
	if second.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("second message method = %q, want textDocument/publishDiagnostics", second.Method)
	}
	var params struct {
		URI         string `json:"uri"`
		Diagnostics []struct {
			Range struct {
				Start struct {
					Line, Character int
				} `json:"start"`
			} `json:"range"`
			Severity int    `json:"severity"`
			Code     string `json:"code"`
			Source   string `json:"source"`
			Message  string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(second.Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params.URI != uri {
		t.Errorf("uri = %q, want %q", params.URI, uri)
	}
	if len(params.Diagnostics) != 1 {
		t.Fatalf("diagnostics count = %d, want 1: %s", len(params.Diagnostics), second.Params)
	}
	d := params.Diagnostics[0]
	if d.Code != "random-seed-not-set" {
		t.Errorf("code = %q, want random-seed-not-set", d.Code)
	}
	if d.Source != "datalint" {
		t.Errorf("source = %q, want datalint", d.Source)
	}
	if d.Severity != 2 { // warning
		t.Errorf("severity = %d, want 2", d.Severity)
	}
	if d.Range.Start.Line != 2 { // 1-based line 3 → 0-based 2
		t.Errorf("range.start.line = %d, want 2", d.Range.Start.Line)
	}
	if !strings.Contains(d.Message, "random.shuffle") {
		t.Errorf("message missing call name: %q", d.Message)
	}
}

func TestServer_ShutdownRequest(t *testing.T) {
	client, serverIn, out := newScriptedClient()
	done := make(chan error, 1)
	go func() {
		done <- lsp.Run(serverIn, out, config.Default())
	}()

	id := json.RawMessage(`7`)
	client.send(t, &lsp.Message{JSONRPC: "2.0", ID: &id, Method: "shutdown"})
	client.send(t, &lsp.Message{JSONRPC: "2.0", Method: "exit"})
	_ = client.in.Close()
	if err := <-done; err != nil && !errors.Is(err, lsp.ErrShutdown) {
		t.Fatalf("Run returned %v", err)
	}

	resp := readOnly(t, out)
	if resp.ID == nil || string(*resp.ID) != "7" {
		t.Errorf("shutdown response id = %v, want 7", resp.ID)
	}
	if string(resp.Result) != "null" {
		t.Errorf("shutdown result = %s, want null", resp.Result)
	}
}

// readOnly reads the first message from out. Tests that send a
// single request expect a single response.
func readOnly(t *testing.T, out *bytes.Buffer) *lsp.Message {
	t.Helper()
	m, err := lsp.ReadMessage(bufio.NewReader(out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	return m
}
