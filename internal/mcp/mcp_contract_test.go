package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/takara-ai/miru-code/internal/mcp"
)

func TestSupportedProtocolVersions(t *testing.T) {
	want := []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05", "2024-10-07"}
	got := mcp.SupportedProtocolVersions
	if len(got) != len(want) {
		t.Fatalf("versions=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("versions[%d]=%s want %s", i, got[i], want[i])
		}
	}
}

func TestToolResultShape(t *testing.T) {
	msg := mcp.ToolText("hello")
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	content, ok := parsed["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content=%v", parsed)
	}
	item := content[0].(map[string]any)
	if item["type"] != "text" || item["text"] != "hello" {
		t.Fatalf("item=%v", item)
	}
}

func TestToolsListIncludesCoreTools(t *testing.T) {
	cache := mcp.NewIndexCache(nil, nil)
	srv := mcp.CreateMcpServer(cache, false)
	tools := srv.ToolNames()
	need := map[string]bool{"search": false, "locate": false, "expand": false, "find_related": false, "auth": false}
	for _, name := range tools {
		if _, ok := need[name]; ok {
			need[name] = true
		}
	}
	for name, ok := range need {
		if !ok {
			t.Errorf("missing tool %s (have %v)", name, tools)
		}
	}
}
