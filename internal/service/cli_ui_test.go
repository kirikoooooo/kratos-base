package service

import (
	"bytes"
	"strings"
	"testing"

	datatrace "kratos-demo/internal/data/trace"
)

func TestRenderCLIBlocksWithFence(t *testing.T) {
	ui := newCLIUI(&bytes.Buffer{})
	renderCLIBlocks(ui, "  ", "before\n```go\nfmt.Println(\"hi\")\n```\nafter")
	out := ui.out.(*bytes.Buffer).String()
	if !strings.Contains(out, "fmt.Println") {
		t.Fatalf("missing code body: %s", out)
	}
	if !strings.Contains(out, "go") {
		t.Fatalf("missing language label: %s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Fatalf("missing plain text: %s", out)
	}
}

func TestFormatCLIEventLineTool(t *testing.T) {
	line := formatCLIEventLine(datatrace.DelegationEvent{
		Stage:    "tool_read_file",
		ToolName: "read_file",
		ToolInput: "README.md",
		Agent:    "router",
	}, false)
	if !strings.Contains(line, "read_file") || !strings.Contains(line, "README.md") {
		t.Fatalf("unexpected line: %q", line)
	}
}

func TestFormatCLIEventLineStepProgress(t *testing.T) {
	line := formatCLIEventLine(datatrace.DelegationEvent{
		Stage:   "step_progress",
		Summary: "步骤 1 完成：已读取 README.md",
	}, false)
	if !strings.Contains(line, "步骤 1 完成") {
		t.Fatalf("unexpected line: %q", line)
	}
}

func TestFormatCLIEventLineToolFailure(t *testing.T) {
	line := formatCLIEventLine(datatrace.DelegationEvent{
		Stage:     "tool_write_file",
		ToolName:  "write_file",
		ToolInput: "hello.ps1",
		Error:     "absolute paths are not allowed",
	}, false)
	if !strings.Contains(line, "write_file") || !strings.Contains(line, "hello.ps1") {
		t.Fatalf("unexpected line: %q", line)
	}
}

func TestFormatCLIEventLineSkipsNoise(t *testing.T) {
	if line := formatCLIEventLine(datatrace.DelegationEvent{Stage: "message_received"}, false); line != "" {
		t.Fatalf("expected skip, got %q", line)
	}
}
