package lsp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/lsp"
)

func TestWriteAndReadMessage_Roundtrip(t *testing.T) {
	id := json.RawMessage(`1`)
	original := &lsp.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{"clientInfo":{"name":"test"}}`),
	}

	var buf bytes.Buffer
	if err := lsp.WriteMessage(&buf, original); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	if !strings.Contains(buf.String(), "Content-Length:") {
		t.Errorf("framing missing Content-Length header: %q", buf.String())
	}

	got, err := lsp.ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got.Method != "initialize" {
		t.Errorf("method = %q, want initialize", got.Method)
	}
	if got.ID == nil || string(*got.ID) != "1" {
		t.Errorf("id = %v, want 1", got.ID)
	}
}

func TestReadMessage_MissingContentLength(t *testing.T) {
	body := "no header here\r\n\r\n"
	_, err := lsp.ReadMessage(bufio.NewReader(strings.NewReader(body)))
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
}

func TestReadMessage_BadJSON(t *testing.T) {
	body := "not json"
	framed := "Content-Length: 8\r\n\r\n" + body
	_, err := lsp.ReadMessage(bufio.NewReader(strings.NewReader(framed)))
	if err == nil {
		t.Fatal("expected error for bad JSON body")
	}
}
