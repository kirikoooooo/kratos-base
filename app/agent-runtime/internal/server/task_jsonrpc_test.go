package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
)

type taskRPCStub struct{}

func (taskRPCStub) CreateTask(context.Context, *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error) {
	return &taskv1.TaskReply{TaskID: "task-1", Status: "pending"}, nil
}
func (taskRPCStub) GetTask(context.Context, *taskv1.GetTaskRequest) (*taskv1.TaskReply, error) {
	return &taskv1.TaskReply{TaskID: "task-1", Status: "done"}, nil
}

func TestTaskJSONRPCCreate(t *testing.T) {
	h := newTaskJSONRPCHandler(taskRPCStub{})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tasks.create","params":{"agent":"coder","prompt":"implement"}}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"taskID":"task-1"`)) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
