package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	taskv1 "kratos-demo/api/task/v1"
)

var (
	ErrPromptRequired            = errors.New("prompt is required")
	ErrTaskNotFound              = errors.New("task not found")
	ErrAgentNotSupported         = errors.New("agent is not supported")
	ErrTaskDispatcherUnavailable = errors.New("task dispatcher is unavailable")
)

type TaskStatus string
type TaskAgent string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"

	TaskAgentDefault  TaskAgent = "default"
	TaskAgentGeneric  TaskAgent = "generic"
	TaskAgentRouter   TaskAgent = "router"
	TaskAgentCoder    TaskAgent = "coder"
	TaskAgentReviewer TaskAgent = "reviewer"
)

type Task struct {
	ID        string             `json:"task_id"`
	Agent     TaskAgent          `json:"agent"`
	Prompt    string             `json:"prompt"`
	Status    TaskStatus         `json:"status"`
	Result    *taskv1.TaskResult `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type TaskRepo interface {
	Save(context.Context, *Task) error
	Update(context.Context, *Task) error
	Get(context.Context, string) (*Task, error)
}

type TaskDispatcher interface {
	Dispatch(context.Context, *taskv1.TaskCommand) error
}

type TaskUsecase struct {
	repo       TaskRepo
	dispatcher TaskDispatcher
	seq        atomic.Uint64
}

func NewTaskUsecase(repo TaskRepo, dispatcher TaskDispatcher) *TaskUsecase {
	return &TaskUsecase{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

func (uc *TaskUsecase) Create(ctx context.Context, agent, prompt string) (*Task, error) {
	normalizedAgent := normalizeAgent(agent)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, ErrPromptRequired
	}

	now := time.Now()
	task := &Task{
		ID:        fmt.Sprintf("task-%d", uc.seq.Add(1)),
		Agent:     normalizedAgent,
		Prompt:    prompt,
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.repo.Save(ctx, task); err != nil {
		return nil, err
	}

	if uc.dispatcher == nil {
		return nil, ErrTaskDispatcherUnavailable
	}

	if err := uc.dispatcher.Dispatch(ctx, task.ToCommand()); err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

func (uc *TaskUsecase) Get(ctx context.Context, id string) (*Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrTaskNotFound
	}
	return uc.repo.Get(ctx, id)
}

func normalizeAgent(agent string) TaskAgent {
	agent = strings.TrimSpace(strings.ToLower(agent))
	if agent == "" {
		return TaskAgentDefault
	}
	switch TaskAgent(agent) {
	case TaskAgentDefault, TaskAgentGeneric:
		return TaskAgentDefault
	}
	return TaskAgent(agent)
}

func (a TaskAgent) String() string {
	return string(a)
}

func (a TaskAgent) IsEmpty() bool {
	return strings.TrimSpace(a.String()) == ""
}

func (t *Task) ToCommand() *taskv1.TaskCommand {
	if t == nil {
		return nil
	}
	return &taskv1.TaskCommand{
		TaskID: t.ID,
		Agent:  t.Agent.String(),
		Prompt: t.Prompt,
	}
}

func (t *Task) MarkRunning(at time.Time) {
	if t == nil {
		return
	}
	t.Status = TaskStatusRunning
	t.Error = ""
	t.UpdatedAt = at
}

func (t *Task) MarkFailed(err error, at time.Time) {
	if t == nil {
		return
	}
	t.Status = TaskStatusFailed
	t.Result = nil
	t.UpdatedAt = at
	if err != nil {
		t.Error = err.Error()
		return
	}
	t.Error = ""
}

func (t *Task) MarkDone(result *taskv1.TaskResult, at time.Time) {
	if t == nil {
		return
	}
	t.Status = TaskStatusDone
	t.Result = result
	t.Error = ""
	t.UpdatedAt = at
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	copyTask := *task
	if task.Result != nil {
		result := *task.Result
		copyTask.Result = &result
	}
	return &copyTask
}

var nowFunc = time.Now
