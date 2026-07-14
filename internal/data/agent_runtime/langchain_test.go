package agent

import (
	"context"
	"io"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"

	"github.com/go-kratos/kratos/v2/log"
)

func TestAgentRuntimeSupportsGenericDefaultProfile(t *testing.T) {
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, nil, nil, nil, log.NewStdLogger(io.Discard))

	tests := []struct {
		name  string
		agent public.AgentKind
		want  bool
	}{
		{name: "default profile", agent: public.AgentKindDefault, want: true},
		{name: "generic alias", agent: public.AgentKindGeneric, want: true},
		{name: "router compatibility", agent: public.AgentKindRouter, want: true},
		{name: "coder compatibility", agent: public.AgentKindCoder, want: true},
		{name: "reviewer compatibility", agent: public.AgentKindReviewer, want: true},
		{name: "unknown profile", agent: public.AgentKind("planner"), want: false},
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
		Agent:  string(public.AgentKindDefault),
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

	result, err := runtime.Execute(context.Background(), public.AgentKindReviewer, "请审查代码")

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
