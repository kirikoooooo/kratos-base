package service

import (
	"context"
	"errors"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
)

type AgentRuntimeService struct {
	taskv1.UnimplementedAgentRuntimeServiceServer

	runtime biz.AgentRuntime
}

func NewAgentRuntimeService(runtime biz.AgentRuntime) *AgentRuntimeService {
	return &AgentRuntimeService{runtime: runtime}
}

func (s *AgentRuntimeService) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if s == nil || s.runtime == nil {
		return nil, errors.New("agent runtime service is not available")
	}
	return s.runtime.ReceiveTask(ctx, cmd)
}
