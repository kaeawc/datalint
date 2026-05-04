// Package lsp implements a minimal Language Server Protocol surface
// for datalint: the JSON-RPC framing and the handful of methods
// required to publish diagnostics to an LSP client (initialize,
// didOpen/didSave, shutdown/exit). Larger surface (didChange,
// workspace/* methods, code actions for auto-fixes) is a follow-up.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a JSON-RPC 2.0 envelope. Requests have ID and Method;
// responses have ID + Result or Error; notifications have Method
// and no ID.
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

// ReadMessage parses one Content-Length-framed JSON-RPC message from r.
func ReadMessage(r *bufio.Reader) (*Message, error) {
	contentLength, err := readContentLength(r)
	if err != nil {
		return nil, err
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var m Message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &m, nil
}

// WriteMessage encodes m and writes the Content-Length-framed JSON to w.
func WriteMessage(w io.Writer, m *Message) error {
	if m.JSONRPC == "" {
		m.JSONRPC = "2.0"
	}
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return nil
}

func readContentLength(r *bufio.Reader) (int, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		const prefix = "Content-Length:"
		if strings.HasPrefix(line, prefix) {
			val := strings.TrimSpace(line[len(prefix):])
			n, err := strconv.Atoi(val)
			if err != nil {
				return 0, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return 0, fmt.Errorf("missing Content-Length header")
	}
	return contentLength, nil
}
