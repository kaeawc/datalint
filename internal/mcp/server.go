package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kaeawc/datalint/internal/config"
)

// protocolVersion is the MCP protocol date this server speaks. The
// client picks the version it advertises in initialize; we echo back
// the highest one we know.
const protocolVersion = "2024-11-05"

// Server holds the per-connection state — currently just the config
// and the writer for outgoing messages. The rule pipeline is
// stateless so initialize/tools-list/tools-call don't need shared
// state beyond that.
type Server struct {
	cfg config.Config
	out io.Writer
}

// NewServer wires a Server to write outgoing messages to out.
func NewServer(out io.Writer, cfg config.Config) *Server {
	return &Server{cfg: cfg, out: out}
}

// ErrShutdown is returned by Run when the client closes stdin or
// sends a shutdown notification. Callers exit zero.
var ErrShutdown = errors.New("mcp: shutdown")

// Run reads JSON-RPC messages from in until EOF or shutdown,
// dispatching each one. Errors from the wire are propagated.
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
			if errors.Is(err, ErrShutdown) {
				return nil
			}
			return err
		}
	}
}

func (s *Server) dispatch(m *Message) error {
	if m.ID == nil {
		// Notification. v0 only acts on shutdown; everything else
		// is a no-op (the spec is permissive about extra notifications).
		if m.Method == "notifications/cancelled" || m.Method == "shutdown" {
			return ErrShutdown
		}
		return nil
	}
	switch m.Method {
	case "initialize":
		return s.respondInitialize(m)
	case "tools/list":
		return s.respondToolsList(m)
	case "tools/call":
		return s.respondToolsCall(m)
	}
	return s.respond(m, nil, &RPCError{
		Code:    -32601,
		Message: fmt.Sprintf("method not found: %s", m.Method),
	})
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

// initializeResult mirrors the MCP InitializeResult shape. v0
// advertises the tools capability only.
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools toolsCapability `json:"tools"`
}

type toolsCapability struct {
	// listChanged is the only capability flag MCP defines today;
	// false means the tool list is static for the connection.
	ListChanged bool `json:"listChanged"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) respondInitialize(m *Message) error {
	result := initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCapabilities{Tools: toolsCapability{ListChanged: false}},
		ServerInfo:      serverInfo{Name: "datalint-mcp", Version: "dev"},
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}
