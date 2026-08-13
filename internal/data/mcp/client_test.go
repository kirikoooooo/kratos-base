package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	protocol "github.com/mark3labs/mcp-go/mcp"
)

func TestFormatToolResultReturnsTextAndError(t *testing.T) {
	output, err := formatToolResult(&protocol.CallToolResult{
		Content: []protocol.Content{protocol.TextContent{Type: "text", Text: "two matches"}},
	})
	if err != nil || output != "two matches" {
		t.Fatalf("formatToolResult() = (%q, %v), want (two matches, nil)", output, err)
	}

	output, err = formatToolResult(&protocol.CallToolResult{
		Content: []protocol.Content{protocol.TextContent{Type: "text", Text: "tool failed"}},
		IsError: true,
	})
	if err == nil || output != "tool failed" {
		t.Fatalf("formatToolResult() = (%q, %v), want (tool failed, error)", output, err)
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
			"--directory", filepath.Join("remote_service", "mcp-server-milvus"),
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
		if tool.OfFunction != nil && tool.OfFunction.Name == "milvus_list_collections" {
			found = true
		}
	}
	if !found {
		t.Fatalf("official Milvus tools = %#v, want milvus_list_collections", tools)
	}
}
