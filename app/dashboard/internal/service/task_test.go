package service

import (
	"context"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/app/dashboard/internal/biz"
)

type fakeTaskClient struct {
	createRequest *taskv1.CreateTaskRequest
}

func (f *fakeTaskClient) CreateTask(_ context.Context, request *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error) {
	f.createRequest = request
	return &taskv1.TaskReply{TaskID: "task-1", Agent: request.GetAgent()}, nil
}

func (*fakeTaskClient) GetTask(context.Context, *taskv1.GetTaskRequest) (*taskv1.TaskReply, error) {
	return &taskv1.TaskReply{TaskID: "task-1"}, nil
}

var _ biz.TaskClient = (*fakeTaskClient)(nil)

func TestTaskServiceUsesRuntimePort(t *testing.T) {
	client := new(fakeTaskClient)
	service := NewTaskService(client)
	reply, err := service.CreateTask(context.Background(), &taskv1.CreateTaskRequest{Agent: "coder", Prompt: "write tests"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.GetTaskID() != "task-1" || client.createRequest.GetPrompt() != "write tests" {
		t.Fatalf("unexpected task forwarding: reply=%+v request=%+v", reply, client.createRequest)
	}
}
