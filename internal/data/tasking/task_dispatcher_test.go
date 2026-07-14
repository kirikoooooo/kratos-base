package tasking

import (
	"context"
	"io"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	dataagent "kratos-demo/internal/data/agent_runtime"
	datatrace "kratos-demo/internal/data/trace"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
)

func TestTaskDispatcherDispatchViaActor(t *testing.T) {
	dataagent.WithFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	repo := NewTaskRepo(logger)
	trace := datatrace.NewDelegationTraceStore()
	runtime := dataagent.NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	dispatcher := NewTaskDispatcher(repo, runtime, trace, nil, logger)

	task := &Task{
		ID:        "task-dispatch",
		Agent:     public.AgentKindReviewer,
		Prompt:    "review boundary conditions",
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch task failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := repo.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("get task failed: %v", err)
		}
		if stored.Status == TaskStatusDone {
			if stored.Result == nil {
				t.Fatal("expected task result after actor dispatch")
			}
			if stored.Result.Summary == "" {
				t.Fatal("expected non-empty task result summary after actor dispatch")
			}
			return
		}
		if stored.Status == TaskStatusFailed {
			t.Fatalf("expected task done, got failed: %s", stored.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for actor-dispatched task to complete")
}

func TestTaskFlowWritesResultAndTrace(t *testing.T) {
	dataagent.WithFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	repo := NewTaskRepo(logger)
	trace := datatrace.NewDelegationTraceStore()
	runtime := dataagent.NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	dispatcher := NewTaskDispatcher(repo, runtime, trace, nil, logger)

	task := &Task{
		ID:        "task-flow",
		Agent:     public.AgentKindDefault,
		Prompt:    "read README.md and summarize phase-one goals",
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch task failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := repo.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("get task failed: %v", err)
		}
		if stored.Status == TaskStatusDone {
			if stored.Result == nil {
				t.Fatal("expected task result")
			}
			if stored.Result.GetSummary() == "" || stored.Result.GetOutput() == "" {
				t.Fatalf("unexpected task result: %+v", stored.Result)
			}
			sessions := trace.ListSessions(1)
			if len(sessions) != 1 {
				t.Fatalf("trace sessions = %d, want 1", len(sessions))
			}
			if sessions[0].Status != string(TaskStatusDone) {
				t.Fatalf("trace status = %q, want done", sessions[0].Status)
			}
			if sessions[0].ResultSummary == "" || sessions[0].ResultOutput == "" {
				t.Fatalf("trace result not written back: %+v", sessions[0])
			}
			return
		}
		if stored.Status == TaskStatusFailed {
			t.Fatalf("expected task done, got failed: %s", stored.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for task flow to complete")
}

func TestDispatchUsesTaskCommand(t *testing.T) {
	task := &Task{
		ID:     "task-cmd",
		Agent:  public.AgentKindDefault,
		Prompt: "hello",
	}
	cmd := taskToCommand(task)
	if cmd.GetTaskID() != task.ID {
		t.Fatalf("task id = %q, want %q", cmd.GetTaskID(), task.ID)
	}
	if cmd.GetAgent() != string(task.Agent) {
		t.Fatalf("agent = %q, want %q", cmd.GetAgent(), task.Agent)
	}
	if cmd.GetPrompt() != task.Prompt {
		t.Fatalf("prompt = %q, want %q", cmd.GetPrompt(), task.Prompt)
	}
	_ = taskv1.TaskCommand{}
}
