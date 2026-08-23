package mcp

import (
	"github.com/takara-ai/miru-code/internal/types"
)

// ServeOptions configures ServeMcp.
type ServeOptions struct {
	Ref       *string
	Content   []types.ContentType
	Benchmark bool
}

// ServeMcp starts the Miru MCP server on stdio.
func ServeMcp(opts ServeOptions) error {
	content := opts.Content
	if len(content) == 0 {
		content = types.DefaultContentTypesCopy()
	}
	cache := NewIndexCache(content, opts.Ref)
	server := CreateMcpServer(cache, opts.Benchmark)
	transport := NewStdioTransport()
	return server.Connect(transport)
}
