package service

import (
	"context"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
)

type AgentRuntimeService struct {
	taskv1.UnimplementedAgentRuntimeServiceServer

	uc *biz.AgentRuntimeUsecase
}

func NewAgentRuntimeService(uc *biz.AgentRuntimeUsecase) *AgentRuntimeService {
	return &AgentRuntimeService{uc: uc}
}

func (s *AgentRuntimeService) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return s.uc.ExecuteTask(ctx, cmd)
}
