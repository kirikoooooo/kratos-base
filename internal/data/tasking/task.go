package tasking

import (
	"context"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	errconst "kratos-demo/internal/consts/error"
	"kratos-demo/internal/consts/public"
)

var (
	ErrPromptRequired            = errconst.ErrPromptRequired
	ErrTaskNotFound              = errconst.ErrTaskNotFound
	ErrTaskDispatcherUnavailable = errconst.ErrTaskDispatcherUnavailable
)

type TaskStatus = public.TaskStatus

const (
	TaskStatusPending = public.TaskStatusPending
	TaskStatusRunning = public.TaskStatusRunning
	TaskStatusDone    = public.TaskStatusDone
	TaskStatusFailed  = public.TaskStatusFailed
)

type Task struct {
	ID        string             `json:"task_id"`
	Agent     public.AgentKind   `json:"agent"`
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
