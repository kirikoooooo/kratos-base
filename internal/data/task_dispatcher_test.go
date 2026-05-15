package data

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/service"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
	grpcserver "google.golang.org/grpc"
)

func TestAgentRuntimeSendTaskViaActor(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, logger)
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
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, logger)
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

func TestDispatchSubTaskViaRemoteGRPC(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	remoteRuntime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, logger)
	remoteService := service.NewAgentRuntimeService(remoteRuntime)

	grpcSrv := grpcserver.NewServer()
	taskv1.RegisterAgentRuntimeServiceServer(grpcSrv, remoteService)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen grpc server failed: %v", err)
	}
	defer listener.Close()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- grpcSrv.Serve(listener)
	}()
	defer func() {
		grpcSrv.Stop()
		select {
		case err := <-serveDone:
			if err != nil {
				t.Fatalf("grpc server stopped with error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting grpc server to stop")
		}
	}()

	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{
		Remotes: []*conf.Runtime_RemoteAgent{{
			Agent:   biz.TaskAgentCoder.String(),
			Target:  listener.Addr().String(),
			Timeout: 3,
		}},
	}, logger)

	result, err := runtime.(*langChainAgentRuntime).dispatchSubTask(context.Background(), biz.TaskAgentCoder, "远程实现一个最小接口")
	if err != nil {
		t.Fatalf("dispatch remote grpc sub task failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil remote task result")
	}
	if result.GetSummary() == "" {
		t.Fatal("expected non-empty remote task result summary")
	}
	if result.GetOutput() == "" {
		t.Fatal("expected non-empty remote task result output")
	}
	if result.GetSummary() != "coder agent 已生成初始方案" {
		t.Fatalf("unexpected remote task summary: %s", result.GetSummary())
	}
}
