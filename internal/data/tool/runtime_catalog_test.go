package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kratos-demo/internal/consts/public"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	datatrace "kratos-demo/internal/data/trace"
)

func TestToolCatalogReadWriteAndCommandTools(t *testing.T) {
	trace := datatrace.NewDelegationTraceStore()
	tools, err := NewRuntime(trace, nil)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	tools.root = dir
	ctx := agentctx.WithAgent(agentctx.WithTaskID(context.Background(), "tool-task"), public.AgentKindDefault)

	writeInput, _ := json.Marshal(map[string]any{
		"path":    "notes/demo.txt",
		"content": "hello\nworld",
	})
	writeOut, err := tools.writeFile(ctx, string(writeInput))
	if err != nil {
		t.Fatalf("writeFile() error = %v", err)
	}
	if !strings.Contains(writeOut, "notes/demo.txt") {
		t.Fatalf("writeFile() output = %q", writeOut)
	}

	readInput, _ := json.Marshal(map[string]any{
		"path":  "notes/demo.txt",
		"start": 1,
		"end":   2,
	})
	readOut, err := tools.readFile(ctx, string(readInput))
	if err != nil {
		t.Fatalf("readFile() error = %v", err)
	}
	if !strings.Contains(readOut, "1: hello") || !strings.Contains(readOut, "2: world") {
		t.Fatalf("readFile() output = %q", readOut)
	}

	cmd := "cat notes/demo.txt"
	if runtime.GOOS == "windows" {
		cmd = "Get-Content notes\\demo.txt"
	}
	cmdOut, err := tools.execCommand(ctx, cmd)
	if err != nil {
		t.Fatalf("execCommand() error = %v", err)
	}
	if !strings.Contains(cmdOut, "hello") || !strings.Contains(cmdOut, "world") {
		t.Fatalf("execCommand() output = %q", cmdOut)
	}

	sessions := trace.ListSessions(1)
	if len(sessions) != 1 {
		t.Fatalf("trace sessions = %d, want 1", len(sessions))
	}
	stages := make([]string, 0, len(sessions[0].Events))
	for _, event := range sessions[0].Events {
		stages = append(stages, event.Stage)
	}
	joined := strings.Join(stages, ",")
	if !strings.Contains(joined, "tool_write_file") || !strings.Contains(joined, "tool_read_file") || !strings.Contains(joined, "tool_exec_command") {
		t.Fatalf("tool event stages = %q", joined)
	}

	if _, err := os.Stat(filepath.Join(dir, "notes", "demo.txt")); err != nil {
		t.Fatalf("expected written file to exist: %v", err)
	}
}

func TestReadFileReturnsDirectoryListing(t *testing.T) {
	trace := datatrace.NewDelegationTraceStore()
	tools, err := NewRuntime(trace, nil)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	if err := os.MkdirAll(filepath.Join(dir, "configs", "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "app.yaml"), []byte("name: demo"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "z-last.txt"), []byte("tail"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tools.root = dir
	ctx := agentctx.WithAgent(agentctx.WithTaskID(context.Background(), "tool-dir-task"), public.AgentKindDefault)

	output, err := tools.readFile(ctx, `{"path":"configs","start":1,"end":10}`)
	if err != nil {
		t.Fatalf("readFile(directory) error = %v", err)
	}
	if !strings.Contains(output, "type: directory") {
		t.Fatalf("directory output missing type marker: %q", output)
	}
	if !strings.Contains(output, "app.yaml") || !strings.Contains(output, "nested/") || !strings.Contains(output, "z-last.txt") {
		t.Fatalf("directory output missing entries: %q", output)
	}
}
