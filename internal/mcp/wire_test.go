package mcp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/mcp"
)

func TestWriteAndReadMessage_Roundtrip(t *testing.T) {
	id := json.RawMessage(`1`)
	original := &mcp.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
	}

	var buf bytes.Buffer
	if err := mcp.WriteMessage(&buf, original); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("framing missing trailing newline: %q", buf.String())
	}
	if strings.Contains(buf.String(), "Content-Length") {
		t.Errorf("MCP framing should not have LSP-style Content-Length header: %q", buf.String())
	}

	got, err := mcp.ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.Method != "initialize" {
		t.Errorf("method = %q, want initialize", got.Method)
	}
}

func TestReadMessage_BadJSON(t *testing.T) {
	_, err := mcp.ReadMessage(bufio.NewReader(strings.NewReader("not json\n")))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestReadMessage_EOFOnEmptyStream(t *testing.T) {
	_, err := mcp.ReadMessage(bufio.NewReader(strings.NewReader("")))
	if err == nil {
		t.Fatal("expected EOF on empty stream")
	}
}
