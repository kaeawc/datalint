package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/pipeline"

	// Register the built-in rule set; the LSP server runs the same
	// pipeline as the CLI so the same rules apply.
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

// Server holds the per-connection state. It's intentionally tiny:
// the rule pipeline is stateless, so the only thing the server
// tracks is the initialized flag (rejects work before initialize)
// and the config it loads at startup.
type Server struct {
	initialized bool
	cfg         config.Config
	out         io.Writer
}

// NewServer returns a Server wired to write outgoing notifications
// (publishDiagnostics) to out. cfg is the config used for every lint
// pass.
func NewServer(out io.Writer, cfg config.Config) *Server {
	return &Server{out: out, cfg: cfg}
}

// ErrShutdown is returned by Run when the client sent the exit
// notification, signaling a clean shutdown. Callers exit zero.
var ErrShutdown = errors.New("lsp: shutdown")

// Run reads JSON-RPC messages from in until the exit notification or
// io.EOF, dispatching each one. Errors from the wire are propagated
// to the caller; ErrShutdown indicates an orderly client-driven exit.
func Run(in io.Reader, out io.Writer, cfg config.Config) error {
	s := NewServer(out, cfg)
	r := bufio.NewReader(in)
	for {
		msg, err := ReadMessage(r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.dispatch(msg); err != nil {
			return err
		}
	}
}

func (s *Server) dispatch(m *Message) error {
	if m.ID != nil {
		return s.handleRequest(m)
	}
	return s.handleNotification(m)
}

func (s *Server) handleRequest(m *Message) error {
	switch m.Method {
	case "initialize":
		return s.respondInitialize(m)
	case "textDocument/codeAction":
		return s.respondCodeAction(m)
	case "shutdown":
		return s.respond(m, json.RawMessage("null"), nil)
	}
	return s.respond(m, nil, &RPCError{
		Code:    -32601,
		Message: fmt.Sprintf("method not found: %s", m.Method),
	})
}

func (s *Server) handleNotification(m *Message) error {
	switch m.Method {
	case "initialized":
		s.initialized = true
		return nil
	case "textDocument/didOpen", "textDocument/didSave":
		return s.lintDocument(m)
	case "textDocument/didClose":
		return s.clearDiagnostics(m)
	case "exit":
		return ErrShutdown
	}
	return nil
}

// initializeResult is the subset of the LSP InitializeResult struct
// we populate. Capabilities advertise that we accept didSave and
// didOpen with full text sync (1) — clients then know to send those
// notifications.
type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	TextDocumentSync   int  `json:"textDocumentSync"` // 1 = Full
	DiagnosticProvider bool `json:"diagnosticProvider,omitempty"`
	CodeActionProvider bool `json:"codeActionProvider,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) respondInitialize(m *Message) error {
	result := initializeResult{
		Capabilities: serverCapabilities{TextDocumentSync: 1, DiagnosticProvider: true, CodeActionProvider: true},
		ServerInfo:   serverInfo{Name: "datalint-lsp", Version: "dev"},
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

func (s *Server) respond(req *Message, result json.RawMessage, rpcErr *RPCError) error {
	resp := &Message{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}
	return WriteMessage(s.out, resp)
}

// textDocumentParams is the subset we need from didOpen/didSave/didClose.
type textDocumentParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

func (s *Server) lintDocument(m *Message) error {
	uri := parseURI(m.Params)
	if uri == "" {
		return nil
	}
	path := uriToPath(uri)
	if path == "" {
		return nil
	}
	findings, runErr := pipeline.Run([]string{path}, s.cfg)
	if runErr != nil {
		return s.publishDiagnostics(uri, nil)
	}
	return s.publishDiagnostics(uri, toLSPDiagnostics(findings, path))
}

func (s *Server) clearDiagnostics(m *Message) error {
	uri := parseURI(m.Params)
	if uri == "" {
		return nil
	}
	return s.publishDiagnostics(uri, nil)
}

// parseURI returns the textDocument.uri from didOpen/didSave/didClose
// params or "" if the params are malformed or the URI is empty.
// Errors are not surfaced — the LSP spec wants servers to be tolerant
// of unexpected client traffic, and the alternative is killing the
// session over a single bad notification.
func parseURI(raw json.RawMessage) string {
	var p textDocumentParams
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	return p.TextDocument.URI
}

// uriToPath strips a file:// scheme. Non-file URIs and parse errors
// yield "" so rules don't accidentally try to read a remote URL.
func uriToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Path
}
