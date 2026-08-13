package biz

import (
	"context"

	taskv1 "kratos-demo/api/task/v1"
)

// TaskClient is Dashboard's only write/read dependency on agent-runtime.
// Dashboard must not import the runtime's implementation packages.
type TaskClient interface {
	CreateTask(context.Context, *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error)
	GetTask(context.Context, *taskv1.GetTaskRequest) (*taskv1.TaskReply, error)
}
