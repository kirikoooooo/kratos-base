package service

import (
	"context"
	"time"

	"kratos-demo/internal/biz"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewTaskService)

type TaskService struct {
	uc *biz.TaskUsecase
}

type CreateTaskRequest struct {
	Agent  string `json:"agent"`
	Prompt string `json:"prompt"`
}

type TaskReply struct {
	TaskID    string          `json:"task_id"`
	Agent     biz.TaskAgent   `json:"agent"`
	Prompt    string          `json:"prompt"`
	Status    biz.TaskStatus  `json:"status"`
	Result    *biz.TaskResult `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func NewTaskService(uc *biz.TaskUsecase) *TaskService {
	return &TaskService{uc: uc}
}

func (s *TaskService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*TaskReply, error) {
	task, err := s.uc.Create(ctx, req.Agent, req.Prompt)
	if err != nil {
		return nil, err
	}
	return toTaskReply(task), nil
}

func (s *TaskService) GetTask(ctx context.Context, id string) (*TaskReply, error) {
	task, err := s.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toTaskReply(task), nil
}

func toTaskReply(task *biz.Task) *TaskReply {
	if task == nil {
		return nil
	}
	return &TaskReply{
		TaskID:    task.ID,
		Agent:     task.Agent,
		Prompt:    task.Prompt,
		Status:    task.Status,
		Result:    task.Result,
		Error:     task.Error,
		CreatedAt: task.CreatedAt.Format(time.RFC3339),
		UpdatedAt: task.UpdatedAt.Format(time.RFC3339),
	}
}
