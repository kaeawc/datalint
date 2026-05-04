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

// Server holds the per-connection state. The rule pipeline is
// stateless, so the only thing the server tracks beyond config is
// the initialized flag and the in-memory document store: open
// editor buffers may diverge from the on-disk file between save
// events, and didChange-driven live linting needs the in-memory
// version.
type Server struct {
	initialized bool
	cfg         config.Config
	out         io.Writer
	docs        map[string][]byte // uri → most recent buffer text
}

// NewServer returns a Server wired to write outgoing notifications
// (publishDiagnostics) to out. cfg is the config used for every lint
// pass.
func NewServer(out io.Writer, cfg config.Config) *Server {
	return &Server{out: out, cfg: cfg, docs: map[string][]byte{}}
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
	case "textDocument/didOpen":
		return s.didOpen(m)
	case "textDocument/didChange":
		return s.didChange(m)
	case "textDocument/didSave":
		return s.didSave(m)
	case "textDocument/didClose":
		return s.didClose(m)
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

// textDocumentParams is the URI-only subset used by didSave / didClose.
type textDocumentParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

// didOpenParams carries the initial buffer text. The store is keyed
// by URI so subsequent didChange notifications can update in place.
type didOpenParams struct {
	TextDocument struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"textDocument"`
}

// didChangeParams carries one or more contentChanges. With full sync
// (the only mode v0 advertises), each entry is the entire new buffer
// text and the server replaces the stored document. Range-based
// (incremental) entries are ignored — they require maintaining an
// edit-applying model that's a follow-up.
type didChangeParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

func (s *Server) didOpen(m *Message) error {
	p, ok := decodeDidOpen(m.Params)
	if !ok || p.TextDocument.URI == "" {
		return nil
	}
	s.docs[p.TextDocument.URI] = []byte(p.TextDocument.Text)
	return s.lintBuffer(p.TextDocument.URI)
}

func (s *Server) didChange(m *Message) error {
	p, ok := decodeDidChange(m.Params)
	if !ok || p.TextDocument.URI == "" || len(p.ContentChanges) == 0 {
		return nil
	}
	// Full sync: take the last change's text as the new buffer.
	s.docs[p.TextDocument.URI] = []byte(p.ContentChanges[len(p.ContentChanges)-1].Text)
	return s.lintBuffer(p.TextDocument.URI)
}

// decodeDidOpen / decodeDidChange parse the params with a (struct,
// bool) return so the caller can early-return on malformed input
// without binding a json error variable in the same statement (which
// nilerr flags as suspicious).
func decodeDidOpen(raw json.RawMessage) (didOpenParams, bool) {
	var p didOpenParams
	if json.Unmarshal(raw, &p) != nil {
		return didOpenParams{}, false
	}
	return p, true
}

func decodeDidChange(raw json.RawMessage) (didChangeParams, bool) {
	var p didChangeParams
	if json.Unmarshal(raw, &p) != nil {
		return didChangeParams{}, false
	}
	return p, true
}

func (s *Server) didSave(m *Message) error {
	uri := parseURI(m.Params)
	if uri == "" {
		return nil
	}
	// On save the on-disk file matches the buffer; clear the in-memory
	// override so subsequent lints (e.g. corpus rules) use disk.
	delete(s.docs, uri)
	return s.lintBuffer(uri)
}

func (s *Server) didClose(m *Message) error {
	uri := parseURI(m.Params)
	if uri == "" {
		return nil
	}
	delete(s.docs, uri)
	return s.publishDiagnostics(uri, nil)
}

// lintBuffer runs the rule pipeline against either the in-memory
// document text (Python) or the on-disk file, and publishes the
// resulting diagnostics under the same URI.
func (s *Server) lintBuffer(uri string) error {
	path := uriToPath(uri)
	if path == "" {
		return nil
	}
	source := s.docs[uri] // nil for files not in the store; pipeline falls back to disk
	findings, runErr := pipeline.RunDocument(path, source, s.cfg)
	if runErr != nil {
		return s.publishDiagnostics(uri, nil)
	}
	return s.publishDiagnostics(uri, toLSPDiagnostics(findings, path))
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
