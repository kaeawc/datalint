package mcp_test

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
	"github.com/kaeawc/datalint/internal/mcp"
)

type scriptedClient struct {
	in *io.PipeWriter
}

func newScriptedClient() (*scriptedClient, *io.PipeReader, *bytes.Buffer) {
	pr, pw := io.Pipe()
	return &scriptedClient{in: pw}, pr, &bytes.Buffer{}
}

func (c *scriptedClient) send(t *testing.T, m *mcp.Message) {
	t.Helper()
	if err := mcp.WriteMessage(c.in, m); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func runOnce(t *testing.T, msgs []*mcp.Message) []*mcp.Message {
	t.Helper()
	client, serverIn, out := newScriptedClient()
	done := make(chan error, 1)
	go func() {
		done <- mcp.Run(serverIn, out, config.Default())
	}()
	for _, m := range msgs {
		client.send(t, m)
	}
	_ = client.in.Close()
	if err := <-done; err != nil && !errors.Is(err, mcp.ErrShutdown) {
		t.Fatalf("Run returned %v", err)
	}

	r := bufio.NewReader(out)
	var responses []*mcp.Message
	for {
		m, err := mcp.ReadMessage(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		responses = append(responses, m)
	}
	return responses
}

func TestServer_InitializeHandshake(t *testing.T) {
	id := json.RawMessage(`1`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "initialize", Params: json.RawMessage(`{}`)},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	resp := resps[0]
	if resp.ID == nil || string(*resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}

	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools struct {
				ListChanged bool `json:"listChanged"`
			} `json:"tools"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.ProtocolVersion == "" {
		t.Errorf("protocolVersion should be non-empty")
	}
	if result.ServerInfo.Name != "datalint-mcp" {
		t.Errorf("server name = %q, want datalint-mcp", result.ServerInfo.Name)
	}
}

func TestServer_ToolsListReportsLint(t *testing.T) {
	id := json.RawMessage(`2`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "tools/list", Params: json.RawMessage(`{}`)},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(result.Tools))
	}
	if result.Tools[0].Name != "lint" {
		t.Errorf("tool name = %q, want lint", result.Tools[0].Name)
	}
	if _, ok := result.Tools[0].InputSchema.Properties["paths"]; !ok {
		t.Errorf("inputSchema missing 'paths' property")
	}
}

func TestServer_ToolsCallLintReturnsFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.py")
	if err := os.WriteFile(path, []byte("import random\n\nrandom.shuffle(data)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	id := json.RawMessage(`3`)
	args, _ := json.Marshal(map[string]any{"paths": []string{path}})
	params, _ := json.Marshal(map[string]any{"name": "lint", "arguments": json.RawMessage(args)})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "tools/call", Params: params},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.IsError {
		t.Fatalf("isError = true, content = %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(result.Content))
	}
	if !strings.Contains(result.Content[0].Text, "random-seed-not-set") {
		t.Errorf("expected random-seed-not-set finding in text:\n%s", result.Content[0].Text)
	}
}

func TestServer_ToolsCallUnknownTool(t *testing.T) {
	id := json.RawMessage(`4`)
	params, _ := json.Marshal(map[string]any{"name": "nope"})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "tools/call", Params: params},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.IsError {
		t.Errorf("isError = false, want true")
	}
	if !strings.Contains(result.Content[0].Text, "unknown tool") {
		t.Errorf("expected 'unknown tool' message: %q", result.Content[0].Text)
	}
}

func TestServer_UnknownMethodReturnsRPCError(t *testing.T) {
	id := json.RawMessage(`5`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "nope/missing", Params: json.RawMessage(`{}`)},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatalf("expected RPC error for unknown method, got %+v", resps[0])
	}
	if resps[0].Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resps[0].Error.Code)
	}
}
