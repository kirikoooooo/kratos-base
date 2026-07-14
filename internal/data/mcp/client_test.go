package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeToolsListCreatesLLMTools(t *testing.T) {
	raw := []byte(`{"tools":[{"name":"rag_search","description":"Search indexed documents","inputSchema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}]}`)

	tools, err := decodeToolsList(raw)
	if err != nil {
		t.Fatalf("decodeToolsList() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Function == nil || tools[0].Function.Name != "rag_search" {
		t.Fatalf("decodeToolsList() = %#v, want rag_search tool", tools)
	}
}

func TestDecodeToolCallReturnsTextAndError(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": "two matches"}},
		"isError": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := decodeToolCall(raw)
	if err != nil || output != "two matches" {
		t.Fatalf("decodeToolCall() = (%q, %v), want (two matches, nil)", output, err)
	}
}

func TestClientDiscoversOfficialMilvusTools(t *testing.T) {
	if os.Getenv("KRATOS_RUN_MCP_INTEGRATION") != "1" {
		t.Skip("set KRATOS_RUN_MCP_INTEGRATION=1 after syncing the official MCP dependencies")
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot(), ".myagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, tools, err := NewClient(ctx, Config{
		Command: "uv",
		Args: []string{
			"--directory", filepath.Join("tools", "mcp-server-milvus"),
			"run", "src/mcp_server_milvus/server.py",
			"--milvus-uri", ".myagent/milvus.db",
		},
		Timeout: 30 * time.Second,
		Env:     []string{"UV_CACHE_DIR=.myagent/uv-cache"},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	found := false
	for _, tool := range tools {
		if tool.Function != nil && tool.Function.Name == "milvus_list_collections" {
			found = true
		}
	}
	if !found {
		t.Fatalf("official Milvus tools = %#v, want milvus_list_collections", tools)
	}
}
