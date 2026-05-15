package service

import (
	"context"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"

	"github.com/google/wire"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ProviderSet = wire.NewSet(NewTaskService)

type TaskService struct {
	taskv1.UnimplementedTaskServiceServer

	uc *biz.TaskUsecase
}

func NewTaskService(uc *biz.TaskUsecase) *TaskService {
	return &TaskService{uc: uc}
}

func (s *TaskService) CreateTask(ctx context.Context, req *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error) {
	task, err := s.uc.Create(ctx, req.Agent, req.Prompt)
	if err != nil {
		return nil, err
	}
	return toTaskReply(task), nil
}

func (s *TaskService) GetTask(ctx context.Context, req *taskv1.GetTaskRequest) (*taskv1.TaskReply, error) {
	task, err := s.uc.Get(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	return toTaskReply(task), nil
}

func toTaskReply(task *biz.Task) *taskv1.TaskReply {
	if task == nil {
		return nil
	}
	return &taskv1.TaskReply{
		TaskID:    task.ID,
		Agent:     task.Agent.String(),
		Prompt:    task.Prompt,
		Status:    string(task.Status),
		Result:    task.Result,
		Error:     task.Error,
		CreatedAt: timestamppb.New(task.CreatedAt),
		UpdatedAt: timestamppb.New(task.UpdatedAt),
	}
}
