package traceexport

import (
	"testing"
	"time"

	datatrace "kratos-demo/internal/data/trace"
)

func TestExtractToolCalls(t *testing.T) {
	events := []datatrace.DelegationEvent{
		{
			Stage:     "tool_read_file",
			ToolName:  "read_file",
			ToolInput: `{"path": "README.md"}`,
			ToolOutput: "path: README.md\n1: # Project",
			Time:     time.Now(),
		},
		{
			Stage:     "tool_exec_command",
			ToolName:  "exec_command",
			ToolInput: `{"command": "go test ./..."}`,
			ToolOutput: "ok  kratos-demo/internal/biz/tool  0.123s",
			ExitCode:  0,
			Time:     time.Now(),
		},
		{
			Stage:     "llm_response",
			ToolName:  "",
			Time:     time.Now(),
		},
		{
			Stage:     "tool_write_file",
			ToolName:  "write_file",
			ToolInput: `{"path": "new.go", "content": "package main"}`,
			ToolOutput: "path: new.go\nbytes_written: 14",
			Time:     time.Now(),
		},
	}

	calls := extractToolCalls(events)

	if len(calls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(calls))
	}

	if calls[0].ToolName != "read_file" {
		t.Errorf("call 0: expected read_file, got %s", calls[0].ToolName)
	}
	if calls[1].ToolName != "exec_command" {
		t.Errorf("call 1: expected exec_command, got %s", calls[1].ToolName)
	}
	if calls[2].ToolName != "write_file" {
		t.Errorf("call 2: expected write_file, got %s", calls[2].ToolName)
	}

	if calls[0].StepIndex != 0 {
		t.Errorf("call 0: expected step_index 0, got %d", calls[0].StepIndex)
	}
	if calls[1].StepIndex != 1 {
		t.Errorf("call 1: expected step_index 1, got %d", calls[1].StepIndex)
	}

	if calls[0].Output == nil {
		t.Error("call 0: expected non-nil output")
	} else if *calls[0].Output != "path: README.md\n1: # Project" {
		t.Errorf("call 0: unexpected output: %s", *calls[0].Output)
	}

	if calls[1].ExitCode == nil {
		t.Error("call 1: expected non-nil exit_code")
	} else if *calls[1].ExitCode != 0 {
		t.Errorf("call 1: expected exit_code 0, got %d", *calls[1].ExitCode)
	}
}

func TestHasToolEvents(t *testing.T) {
	tests := []struct {
		name   string
		events []datatrace.DelegationEvent
		want   bool
	}{
		{
			name:   "empty",
			events: nil,
			want:   false,
		},
		{
			name: "no tool events",
			events: []datatrace.DelegationEvent{
				{Stage: "llm_response"},
				{Stage: "plan_update"},
			},
			want: false,
		},
		{
			name: "has tool events",
			events: []datatrace.DelegationEvent{
				{Stage: "llm_response"},
				{Stage: "tool_read_file", ToolName: "read_file"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasToolEvents(tt.events)
			if got != tt.want {
				t.Errorf("hasToolEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsToolStage(t *testing.T) {
	tests := []struct {
		stage string
		want  bool
	}{
		{"tool_read_file", true},
		{"tool_edit_file", true},
		{"tool_exec_command", true},
		{"tool_write_file", true},
		{"tool_delete_file", true},
		{"tool_search_skills", true},
		{"tool_load_skill", true},
		{"tool_daytona_data_analysis", true},
		{"llm_response", false},
		{"plan_update", false},
		{"context_compress", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			got := isToolStage(tt.stage)
			if got != tt.want {
				t.Errorf("isToolStage(%q) = %v, want %v", tt.stage, got, tt.want)
			}
		})
	}
}
