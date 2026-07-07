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
		agent biz.AgentKind
		want  bool
	}{
		{name: "default profile", agent: biz.AgentKindDefault, want: true},
		{name: "generic alias", agent: biz.AgentKindGeneric, want: true},
		{name: "router compatibility", agent: biz.AgentKindRouter, want: true},
		{name: "coder compatibility", agent: biz.AgentKindCoder, want: true},
		{name: "reviewer compatibility", agent: biz.AgentKindReviewer, want: true},
		{name: "unknown profile", agent: biz.AgentKind("planner"), want: false},
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
		Agent:  string(biz.AgentKindDefault),
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

func TestFakeProviderCreatesLLM(t *testing.T) {
	WithFakeRuntimeLLM(t)

	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, nil, log.NewStdLogger(io.Discard))

	result, err := runtime.Execute(context.Background(), biz.AgentKindReviewer, "请审查代码")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.GetOutput() == "" {
		t.Fatal("expected non-empty task result output")
	}
}
