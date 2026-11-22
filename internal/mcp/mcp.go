// Package mcp implements a Model Context Protocol server over JSON-RPC 2.0.
//
// The server exposes Samplier's query, report, and export capabilities as MCP
// tools so AI agents can run custom reports and exports. Transport is stdio
// (newline-delimited JSON-RPC messages), which is what MCP hosts expect.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// ProtocolVersion is the MCP protocol revision implemented.
const ProtocolVersion = "2024-11-05"

// JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s (code %d)", e.Message, e.Code) }

// Tool is a callable MCP tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	// Handler executes the tool with raw JSON arguments. It is not serialised.
	Handler func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

// Server is an MCP JSON-RPC server.
type Server struct {
	Name    string
	Version string
	tools   map[string]Tool
	order   []string
	logger  *slog.Logger

	mu sync.RWMutex
}

// NewServer constructs an MCP server.
func NewServer(name, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{Name: name, Version: version, tools: map[string]Tool{}, logger: logger}
}

// AddTool registers a tool.
func (s *Server) AddTool(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[t.Name]; !exists {
		s.order = append(s.order, t.Name)
	}
	s.tools[t.Name] = t
}

// Tools returns the registered tools in registration order.
func (s *Server) Tools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tool, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, s.tools[name])
	}
	return out
}

// Handle processes one JSON-RPC request and returns a response. A nil response
// means the message was a notification (no id) and needs no reply.
func (s *Server) Handle(ctx context.Context, req *Request) *Response {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return errResponse(req.ID, CodeInvalidRequest, "jsonrpc must be \"2.0\"")
	}
	if req.Method == "" {
		return errResponse(req.ID, CodeInvalidRequest, "method is required")
	}

	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return okResponse(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    s.Name,
				"version": s.Version,
			},
		})

	case "notifications/initialized", "initialized":
		return nil

	case "ping":
		return okResponse(req.ID, map[string]any{})

	case "tools/list":
		return okResponse(req.ID, map[string]any{"tools": s.Tools()})

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errResponse(req.ID, CodeInvalidParams, "invalid params: "+err.Error())
		}
		if p.Name == "" {
			return errResponse(req.ID, CodeInvalidParams, "tool name is required")
		}
		s.mu.RLock()
		tool, ok := s.tools[p.Name]
		s.mu.RUnlock()
		if !ok {
			return errResponse(req.ID, CodeInvalidParams, "unknown tool: "+p.Name)
		}

		result, err := tool.Handler(ctx, p.Arguments)
		if err != nil {
			// Tool errors are reported as successful calls with isError=true,
			// per the MCP specification, so agents can read the message.
			return okResponse(req.ID, map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": err.Error()},
				},
				"isError": true,
			})
		}

		text, err := resultText(result)
		if err != nil {
			return errResponse(req.ID, CodeInternalError, "encode result: "+err.Error())
		}
		return okResponse(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": false,
		})

	default:
		if isNotification {
			return nil
		}
		return errResponse(req.ID, CodeMethodNotFound, "unknown method: "+req.Method)
	}
}

// resultText renders a tool result as text content.
func resultText(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func okResponse(id json.RawMessage, result any) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Result: result}
}

func errResponse(id json.RawMessage, code int, msg string) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}}
}

// Serve reads newline-delimited JSON-RPC messages from r and writes responses
// to w until EOF or context cancellation.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Allow large export payloads in a single message.
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)

	enc := json.NewEncoder(w)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(errResponse(nil, CodeParseError, "parse error: "+err.Error())); err != nil {
				return err
			}
			continue
		}

		resp := s.Handle(ctx, &req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ErrUnsupported indicates an unsupported operation.
var ErrUnsupported = errors.New("unsupported operation")
