package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func testServer() *Server {
	s := NewServer("samplier", "test", nil)
	s.AddTool(Tool{
		Name:        "echo",
		Description: "Echo the input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"echo": string(args)}, nil
		},
	})
	s.AddTool(Tool{
		Name:        "fail",
		Description: "Always fails",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			return nil, context.DeadlineExceeded
		},
	})
	return s
}

func call(t *testing.T, s *Server, id, method string, params string) *Response {
	t.Helper()
	req := &Request{JSONRPC: "2.0", Method: method}
	if id != "" {
		req.ID = json.RawMessage(`"` + id + `"`)
	}
	if params != "" {
		req.Params = json.RawMessage(params)
	}
	return s.Handle(context.Background(), req)
}

func TestInitialize(t *testing.T) {
	s := testServer()
	resp := call(t, s, "1", "initialize", "")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	res, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", resp.Result)
	}
	if res["protocolVersion"] != ProtocolVersion {
		t.Fatalf("got version %v", res["protocolVersion"])
	}
	info := res["serverInfo"].(map[string]any)
	if info["name"] != "samplier" {
		t.Fatalf("got name %v", info["name"])
	}
}

func TestToolsList(t *testing.T) {
	s := testServer()
	resp := call(t, s, "2", "tools/list", "")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	tools := res["tools"].([]Tool)
	if len(tools) != 2 {
		t.Fatalf("got %d tools", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Fatalf("registration order lost: %v", tools[0].Name)
	}
}

func TestToolsCallSuccess(t *testing.T) {
	s := testServer()
	resp := call(t, s, "3", "tools/call", `{"name":"echo","arguments":{"x":1}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] != false {
		t.Fatal("expected isError=false")
	}
	content := res["content"].([]map[string]any)
	if len(content) != 1 || content[0]["type"] != "text" {
		t.Fatalf("unexpected content: %v", content)
	}
	if !strings.Contains(content[0]["text"].(string), "echo") {
		t.Fatalf("unexpected text: %v", content[0]["text"])
	}
}

func TestToolsCallError(t *testing.T) {
	s := testServer()
	resp := call(t, s, "4", "tools/call", `{"name":"fail","arguments":{}}`)
	if resp.Error != nil {
		t.Fatalf("tool errors should not be RPC errors: %v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] != true {
		t.Fatal("expected isError=true")
	}
	content := res["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "deadline") {
		t.Fatalf("unexpected text: %v", content[0]["text"])
	}
}

func TestUnknownTool(t *testing.T) {
	s := testServer()
	resp := call(t, s, "5", "tools/call", `{"name":"nope","arguments":{}}`)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != CodeInvalidParams {
		t.Fatalf("got code %d", resp.Error.Code)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := testServer()
	resp := call(t, s, "6", "bogus/method", "")
	if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestPing(t *testing.T) {
	s := testServer()
	resp := call(t, s, "7", "ping", "")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	s := testServer()
	if resp := call(t, s, "", "notifications/initialized", ""); resp != nil {
		t.Fatalf("expected nil response for notification, got %+v", resp)
	}
	// Unknown notification also returns nil.
	if resp := call(t, s, "", "unknown/notification", ""); resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
}

func TestInvalidJSONRPCVersion(t *testing.T) {
	s := testServer()
	req := &Request{JSONRPC: "1.0", Method: "ping", ID: json.RawMessage(`"8"`)}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestMissingMethod(t *testing.T) {
	s := testServer()
	req := &Request{JSONRPC: "2.0", ID: json.RawMessage(`"9"`)}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestInvalidParams(t *testing.T) {
	s := testServer()
	resp := call(t, s, "10", "tools/call", `{bad json`)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("unexpected response: %+v", resp)
	}
	// Missing tool name.
	resp = call(t, s, "11", "tools/call", `{"arguments":{}}`)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestServeStream(t *testing.T) {
	s := testServer()
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"1","method":"initialize"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"2","method":"tools/list"}`,
		`not json`,
		``,
		`{"jsonrpc":"2.0","id":"3","method":"ping"}`,
	}, "\n")

	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// initialize, tools/list, parse-error, ping => 4 responses.
	if len(lines) != 4 {
		t.Fatalf("got %d responses: %v", len(lines), lines)
	}

	// The parse error must be reported.
	var foundParseErr bool
	for _, l := range lines {
		var resp Response
		if err := json.Unmarshal([]byte(l), &resp); err != nil {
			t.Fatalf("invalid response JSON: %v", err)
		}
		if resp.Error != nil && resp.Error.Code == CodeParseError {
			foundParseErr = true
		}
	}
	if !foundParseErr {
		t.Fatal("expected a parse error response")
	}
}

func TestServeContextCancel(t *testing.T) {
	s := testServer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	manyLines := strings.Repeat(`{"jsonrpc":"2.0","id":"1","method":"ping"}`+"\n", 10)
	var out bytes.Buffer
	err := s.Serve(ctx, strings.NewReader(manyLines), &out)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestResultTextVariants(t *testing.T) {
	if got, _ := resultText("plain"); got != "plain" {
		t.Errorf("got %q", got)
	}
	if got, _ := resultText([]byte("bytes")); got != "bytes" {
		t.Errorf("got %q", got)
	}
	got, err := resultText(map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, `"a": "b"`) {
		t.Errorf("got %q", got)
	}
}

func TestAddToolOverwrite(t *testing.T) {
	s := NewServer("x", "1", nil)
	s.AddTool(Tool{Name: "a", Handler: func(context.Context, json.RawMessage) (any, error) { return "1", nil }})
	s.AddTool(Tool{Name: "a", Handler: func(context.Context, json.RawMessage) (any, error) { return "2", nil }})
	if len(s.Tools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(s.Tools()))
	}
}

func TestHelpers(t *testing.T) {
	if got := clamp(0, 1, 10, 5); got != 5 {
		t.Errorf("clamp default = %d", got)
	}
	if got := clamp(-3, 1, 10, 5); got != 1 {
		t.Errorf("clamp min = %d", got)
	}
	if got := clamp(99, 1, 10, 5); got != 10 {
		t.Errorf("clamp max = %d", got)
	}
	if got := clamp(7, 1, 10, 5); got != 7 {
		t.Errorf("clamp pass-through = %d", got)
	}
	if got := sanitizeFilename("P-001/a b"); got != "P-001_a_b" {
		t.Errorf("sanitizeFilename = %q", got)
	}
	if got := sanitizeFilename(""); got != "samplier" {
		t.Errorf("sanitizeFilename empty = %q", got)
	}
	if got := deref(nil); got != "" {
		t.Errorf("deref nil = %q", got)
	}
	s := "x"
	if got := deref(&s); got != "x" {
		t.Errorf("deref = %q", got)
	}
	if parseDatePtr(nil) != nil {
		t.Error("expected nil")
	}
	if parseDatePtr(&s) != nil {
		t.Error("expected nil for invalid date")
	}
	d := "2024-01-02"
	if parseDatePtr(&d) == nil {
		t.Error("expected parsed date")
	}
}

func TestSchemaHelper(t *testing.T) {
	got := schema(`{"type":"object","properties":{}}`, []string{"a", "b"})
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("invalid JSON schema: %v (%s)", err, got)
	}
	req, ok := m["required"].([]any)
	if !ok || len(req) != 2 {
		t.Fatalf("unexpected required: %v", m["required"])
	}

	// No required fields: unchanged.
	plain := schema(`{"type":"object"}`, nil)
	if strings.Contains(string(plain), "required") {
		t.Errorf("unexpected required in %s", plain)
	}
}
