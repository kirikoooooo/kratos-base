package data

import (
	"context"
	"io"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
)

func TestAgentRuntimeSendTaskViaActor(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := NewAgentRuntime(&conf.AI{}, logger)
	defer actorpkg.StopActor(runtime.PID())

	_ = NewTaskDispatcher(NewTaskRepo(logger), runtime, logger)

	result, err := runtime.SendTask(context.Background(), &taskv1.TaskCommand{
		TaskID: "task-sync",
		Agent:  biz.TaskAgentCoder.String(),
		Prompt: "实现一个最小示例",
	})

	if err != nil {
		t.Fatalf("expected actor send task success, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.Summary == "" {
		t.Fatal("expected non-empty task result summary")
	}
}

func TestTaskDispatcherDispatchViaActor(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	repo := NewTaskRepo(logger)
	runtime := NewAgentRuntime(&conf.AI{}, logger)
	defer actorpkg.StopActor(runtime.PID())

	dispatcher := NewTaskDispatcher(repo, runtime, logger)

	task := &biz.Task{
		ID:        "task-dispatch",
		Agent:     biz.TaskAgentReviewer,
		Prompt:    "检查边界条件",
		Status:    biz.TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), task.ToCommand()); err != nil {
		t.Fatalf("dispatch task failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := repo.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("get task failed: %v", err)
		}
		if stored.Status == biz.TaskStatusDone {
			if stored.Result == nil {
				t.Fatal("expected task result after actor dispatch")
			}
			if stored.Result.Summary == "" {
				t.Fatal("expected non-empty task result summary after actor dispatch")
			}
			return
		}
		if stored.Status == biz.TaskStatusFailed {
			t.Fatalf("expected task done, got failed: %s", stored.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for actor-dispatched task to complete")
}
