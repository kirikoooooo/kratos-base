package tasking

import (
	"context"
	"errors"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
)

var (
	ErrPromptRequired            = errors.New("prompt is required")
	ErrTaskNotFound              = errors.New("task not found")
	ErrTaskDispatcherUnavailable = errors.New("task dispatcher is unavailable")
)

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
)

type Task struct {
	ID        string             `json:"task_id"`
	Agent     biz.AgentKind          `json:"agent"`
	Prompt    string             `json:"prompt"`
	Status    TaskStatus         `json:"status"`
	Result    *taskv1.TaskResult `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type TaskRepo interface {
	Save(ctx context.Context, task *Task) error
	Update(ctx context.Context, task *Task) error
	Get(ctx context.Context, id string) (*Task, error)
}

type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *Task) error
}
