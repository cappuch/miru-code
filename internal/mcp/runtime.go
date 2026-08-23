package mcp

import (
	"encoding/json"
	"fmt"
	"os"
)

// ToolResult is the MCP tools/call result shape.
type ToolResult struct {
	Content []ToolContent `json:"content"`
}

// ToolContent is a single content block.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolText wraps a string as MCP text content.
func ToolText(content string) ToolResult {
	return ToolResult{Content: []ToolContent{{Type: "text", Text: content}}}
}

// ToolHandler handles a tools/call.
type ToolHandler func(args map[string]any) (ToolResult, error)

// ToolSchema describes a registered tool.
type ToolSchema struct {
	Description string
	InputSchema map[string]any // JSON Schema object
	Handler     ToolHandler
}

// MiruMcpServerInfo is server metadata.
type MiruMcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MiruMcpServer is a minimal MCP server.
type MiruMcpServer struct {
	ServerInfo   MiruMcpServerInfo
	Instructions string
	tools        map[string]ToolSchema
	transport    *StdioTransport
}

// NewMiruMcpServer constructs a server.
func NewMiruMcpServer(info MiruMcpServerInfo, instructions string) *MiruMcpServer {
	return &MiruMcpServer{
		ServerInfo:   info,
		Instructions: instructions,
		tools:        map[string]ToolSchema{},
	}
}

// RegisterTool registers a tool.
func (s *MiruMcpServer) RegisterTool(name string, schema ToolSchema) {
	s.tools[name] = schema
}

// ToolNames returns registered tool names (unsorted).
func (s *MiruMcpServer) ToolNames() []string {
	out := make([]string, 0, len(s.tools))
	for name := range s.tools {
		out = append(out, name)
	}
	return out
}

// Connect binds a transport and serves forever.
func (s *MiruMcpServer) Connect(transport *StdioTransport) error {
	s.transport = transport
	transport.OnMessage = s.handleMessage
	transport.OnError = func(err error) {
		fmt.Fprintln(os.Stderr, err)
	}
	return transport.Start()
}

// Close closes the transport.
func (s *MiruMcpServer) Close() error {
	if s.transport != nil {
		return s.transport.Close()
	}
	return nil
}

func (s *MiruMcpServer) handleMessage(message JsonRpcMessage) error {
	method, _ := message["method"].(string)
	_, hasID := message["id"]
	if method != "" && !hasID {
		// notification
		return nil
	}
	if method == "" || !hasID {
		return nil
	}
	id := message["id"]
	result, err := s.dispatch(method, message["params"])
	if err != nil {
		return s.sendError(id, err)
	}
	return s.transport.Send(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

type rpcError struct {
	code    int
	message string
	data    any
}

func (e *rpcError) Error() string { return e.message }

func rpcErr(code int, message string, data any) *rpcError {
	return &rpcError{code: code, message: message, data: data}
}

const (
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternal       = -32603
)

func (s *MiruMcpServer) dispatch(method string, params any) (any, error) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.listTools()}, nil
	case "tools/call":
		return s.handleToolCall(params)
	default:
		return nil, rpcErr(rpcMethodNotFound, "Method not found: "+method, nil)
	}
}

func (s *MiruMcpServer) handleInitialize(params any) (map[string]any, error) {
	m, _ := params.(map[string]any)
	requested, _ := m["protocolVersion"].(string)
	protocolVersion := LatestProtocolVersion
	for _, v := range SupportedProtocolVersions {
		if v == requested {
			protocolVersion = requested
			break
		}
	}
	out := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    s.getCapabilities(),
		"serverInfo":      s.ServerInfo,
	}
	if s.Instructions != "" {
		out["instructions"] = s.Instructions
	}
	return out, nil
}

func (s *MiruMcpServer) getCapabilities() map[string]any {
	if len(s.tools) == 0 {
		return map[string]any{}
	}
	return map[string]any{"tools": map[string]any{"listChanged": true}}
}

func (s *MiruMcpServer) listTools() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for name, tool := range s.tools {
		out = append(out, map[string]any{
			"name":        name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}
	return out
}

func (s *MiruMcpServer) handleToolCall(params any) (any, error) {
	m, ok := params.(map[string]any)
	if !ok {
		return nil, rpcErr(rpcInvalidParams, "Invalid tools/call params", nil)
	}
	name, _ := m["name"].(string)
	tool, ok := s.tools[name]
	if !ok {
		return nil, rpcErr(rpcInvalidParams, "Tool "+name+" not found", nil)
	}
	args, _ := m["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	result, err := tool.Handler(args)
	if err != nil {
		return ToolText(err.Error()), nil
	}
	return result, nil
}

func (s *MiruMcpServer) sendError(id any, err error) error {
	code := rpcInternal
	msg := err.Error()
	var data any
	if re, ok := err.(*rpcError); ok {
		code = re.code
		msg = re.message
		data = re.data
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	}
	if data != nil {
		payload["error"].(map[string]any)["data"] = data
	}
	return s.transport.Send(payload)
}

// StringArg extracts a string tool argument.
func StringArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// IntArg extracts an optional int tool argument.
func IntArg(args map[string]any, key string) *int {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		n := int(t)
		return &n
	case int:
		return &t
	case json.Number:
		n64, err := t.Int64()
		if err != nil {
			return nil
		}
		n := int(n64)
		return &n
	default:
		return nil
	}
}

// BoolArg extracts an optional bool tool argument.
func BoolArg(args map[string]any, key string) *bool {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

// StringSliceArg extracts a string or string array.
func StringSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// Prop builds a JSON Schema property.
func Prop(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

// ObjectSchema builds a JSON Schema object for tools/list.
func ObjectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
