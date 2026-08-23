package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// JsonRpcID is a JSON-RPC request id.
type JsonRpcID any // string | number

// JsonRpcMessage is any JSON-RPC envelope.
type JsonRpcMessage map[string]any

// SupportedProtocolVersions lists MCP protocol versions Miru accepts.
var SupportedProtocolVersions = []string{
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
	"2024-10-07",
}

// LatestProtocolVersion is the newest supported version.
var LatestProtocolVersion = SupportedProtocolVersions[0]

// StdioTransport is line-delimited JSON-RPC over stdin/stdout.
type StdioTransport struct {
	mu      sync.Mutex
	buffer  string
	started bool
	closed  bool
	reader  io.Reader
	writer  io.Writer

	OnMessage func(message JsonRpcMessage) error
	OnError   func(error)
	OnClose   func()
}

// NewStdioTransport creates a transport bound to os.Stdin/os.Stdout.
func NewStdioTransport() *StdioTransport {
	return &StdioTransport{reader: os.Stdin, writer: os.Stdout}
}

// Start reads stdin until EOF.
func (t *StdioTransport) Start() error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("StdioTransport already started")
	}
	t.started = true
	reader := t.reader
	t.mu.Unlock()

	scanner := bufio.NewScanner(reader)
	// Allow large MCP payloads.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var message JsonRpcMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			if t.OnError != nil {
				t.OnError(err)
			}
			continue
		}
		if t.OnMessage != nil {
			if err := t.OnMessage(message); err != nil && t.OnError != nil {
				t.OnError(err)
			}
		}
	}
	if err := scanner.Err(); err != nil && t.OnError != nil {
		t.OnError(err)
	}
	if t.OnClose != nil {
		t.OnClose()
	}
	return nil
}

// Close marks the transport closed.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	if t.OnClose != nil {
		t.OnClose()
	}
	return nil
}

// Send writes one JSON-RPC message as a single line.
func (t *StdioTransport) Send(message any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(t.writer, "%s\n", data)
	return err
}
