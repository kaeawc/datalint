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
		return s.handleNotification(m)
	}
	if handler := s.requestHandler(m.Method); handler != nil {
		return handler(m)
	}
	return s.respond(m, nil, &RPCError{
		Code:    -32601,
		Message: fmt.Sprintf("method not found: %s", m.Method),
	})
}

// handleNotification covers the notifications v0 acts on. The spec
// is permissive about extra notifications, so anything else is a
// no-op rather than an error.
func (s *Server) handleNotification(m *Message) error {
	if m.Method == "notifications/cancelled" || m.Method == "shutdown" {
		return ErrShutdown
	}
	return nil
}

// requestHandler returns the matching responder for a JSON-RPC
// request. Pulled out of dispatch so the cyclomatic complexity stays
// under the gocyclo 10 limit as the method surface grows.
func (s *Server) requestHandler(method string) func(*Message) error {
	switch method {
	case "initialize":
		return s.respondInitialize
	case "tools/list":
		return s.respondToolsList
	case "tools/call":
		return s.respondToolsCall
	case "resources/list":
		return s.respondResourcesList
	case "resources/read":
		return s.respondResourcesRead
	case "prompts/list":
		return s.respondPromptsList
	case "prompts/get":
		return s.respondPromptsGet
	}
	return nil
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
// advertises the tools and resources capabilities.
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools     toolsCapability     `json:"tools"`
	Resources resourcesCapability `json:"resources"`
	Prompts   promptsCapability   `json:"prompts"`
}

type toolsCapability struct {
	// listChanged is the only capability flag MCP defines today;
	// false means the tool list is static for the connection.
	ListChanged bool `json:"listChanged"`
}

type resourcesCapability struct {
	// listChanged: false → resource set is static. subscribe: false
	// → server doesn't push resources/updated notifications.
	ListChanged bool `json:"listChanged"`
	Subscribe   bool `json:"subscribe"`
}

type promptsCapability struct {
	// listChanged: false → prompt set is static for the connection.
	ListChanged bool `json:"listChanged"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (s *Server) respondInitialize(m *Message) error {
	result := initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities: serverCapabilities{
			Tools:     toolsCapability{ListChanged: false},
			Resources: resourcesCapability{ListChanged: false, Subscribe: false},
			Prompts:   promptsCapability{ListChanged: false},
		},
		ServerInfo: serverInfo{Name: "datalint-mcp", Version: "dev"},
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}
