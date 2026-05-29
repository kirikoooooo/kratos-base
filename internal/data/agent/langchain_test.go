package agent

import (
	"context"
	"io"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func TestAgentRuntimeSupportsGenericDefaultProfile(t *testing.T) {
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, nil, log.NewStdLogger(io.Discard))

	tests := []struct {
		name  string
		agent biz.Agent
		want  bool
	}{
		{name: "default profile", agent: biz.AgentDefault, want: true},
		{name: "generic alias", agent: biz.AgentGeneric, want: true},
		{name: "router compatibility", agent: biz.AgentRouter, want: true},
		{name: "coder compatibility", agent: biz.AgentCoder, want: true},
		{name: "reviewer compatibility", agent: biz.AgentReviewer, want: true},
		{name: "unknown profile", agent: biz.Agent("planner"), want: false},
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
	WithFakeRuntimeLLM(t)

	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, nil, log.NewStdLogger(io.Discard))

	result, err := runtime.ReceiveTask(context.Background(), &taskv1.TaskCommand{
		TaskID: "task-default-profile",
		Agent:  string(biz.AgentDefault),
		Prompt: "???????????",
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
