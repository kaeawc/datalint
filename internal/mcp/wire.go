// Package mcp implements a minimal Model Context Protocol server for
// datalint: stdio transport with line-delimited JSON-RPC 2.0, the
// initialize handshake, and a single `lint` tool that runs the rule
// pipeline on the requested paths.
//
// MCP framing differs from LSP: each message is one JSON object on
// its own line, terminated by '\n'. No Content-Length header.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Message is a JSON-RPC 2.0 envelope. Same shape as LSP — but
// duplicated here to keep the two protocols' wire formats decoupled.
type Message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// RPCError is the JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ReadMessage parses one newline-terminated JSON-RPC message from r.
// io.EOF is returned on a clean stream end.
func ReadMessage(r *bufio.Reader) (*Message, error) {
	line, err := r.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return nil, err
	}
	var m Message
	if jerr := json.Unmarshal(line, &m); jerr != nil {
		return nil, fmt.Errorf("decode message: %w", jerr)
	}
	return &m, nil
}

// WriteMessage encodes m and writes a single line followed by '\n'.
// JSONRPC defaults to "2.0" when empty.
func WriteMessage(w io.Writer, m *Message) error {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		return err
	}
	return nil
}
