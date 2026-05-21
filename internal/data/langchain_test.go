package data

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func TestAgentRuntimeSupportsGenericDefaultProfile(t *testing.T) {
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, log.NewStdLogger(io.Discard))

	tests := []struct {
		name  string
		agent biz.TaskAgent
		want  bool
	}{
		{name: "default profile", agent: biz.TaskAgentDefault, want: true},
		{name: "generic alias", agent: biz.TaskAgentGeneric, want: true},
		{name: "router compatibility", agent: biz.TaskAgentRouter, want: true},
		{name: "coder compatibility", agent: biz.TaskAgentCoder, want: true},
		{name: "reviewer compatibility", agent: biz.TaskAgentReviewer, want: true},
		{name: "unknown profile", agent: biz.TaskAgent("planner"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runtime.Supports(tt.agent); got != tt.want {
				t.Fatalf("Supports(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}

func TestAgentRuntimeReceiveTaskAcceptsDefaultProfile(t *testing.T) {
	withFakeRuntimeLLM(t)

	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, log.NewStdLogger(io.Discard))

	result, err := runtime.ReceiveTask(context.Background(), &taskv1.TaskCommand{
		TaskID: "task-default-profile",
		Agent:  biz.TaskAgentDefault.String(),
		Prompt: "请给出一个最小实现方案",
	})

	if err != nil {
		t.Fatalf("ReceiveTask() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.GetSummary() == "" {
		t.Fatal("expected non-empty task result summary")
	}
	if result.GetOutput() == "" {
		t.Fatal("expected non-empty task result output")
	}
}

func TestToolCatalogReadWriteAndCommandTools(t *testing.T) {
	trace := NewDelegationTraceStore()
	tools, err := newLocalToolRuntime(trace)
	if err != nil {
		t.Fatalf("newLocalToolRuntime() error = %v", err)
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
	ctx := withTaskAgent(withTaskID(context.Background(), "tool-task"), biz.TaskAgentDefault)

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

	cmdOut, err := tools.execCommand(ctx, "Get-Content notes\\demo.txt")
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
	trace := NewDelegationTraceStore()
	tools, err := newLocalToolRuntime(trace)
	if err != nil {
		t.Fatalf("newLocalToolRuntime() error = %v", err)
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
	ctx := withTaskAgent(withTaskID(context.Background(), "tool-dir-task"), biz.TaskAgentDefault)

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

func TestInitialPlanAndAdvance(t *testing.T) {
	plan := initialPlan(biz.TaskAgentDefault, "请读取 README 然后总结")
	if len(plan) < 4 {
		t.Fatalf("initialPlan() steps = %d, want >= 4", len(plan))
	}
	if plan[0].Status != "in_progress" {
		t.Fatalf("first step status = %q, want in_progress", plan[0].Status)
	}

	advanced := advancePlan(plan, biz.DelegationEvent{Stage: "tool_read_file"})
	foundCompleted := false
	for _, step := range advanced {
		if step.ID == "inspect" && step.Status == "completed" {
			foundCompleted = true
		}
	}
	if !foundCompleted {
		t.Fatalf("expected inspect step to complete after tool_read_file: %+v", advanced)
	}
}

func TestNormalizeFunctionCallingModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank", input: "", want: defaultOpenAIModel},
		{name: "gpt5 fallback", input: "gpt-5.4-mini", want: defaultOpenAIModel},
		{name: "compatible model kept", input: "gpt-4o-mini", want: "gpt-4o-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFunctionCallingModel(tt.input); got != tt.want {
				t.Fatalf("normalizeFunctionCallingModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
