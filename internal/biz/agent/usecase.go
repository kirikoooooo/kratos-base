package agent

import (
	"context"
	"errors"

	taskv1 "kratos-demo/api/task/v1"
)

// AgentRuntimeUsecase orchestrates agent task execution.
type AgentRuntimeUsecase struct {
	runtime AgentRuntime
}

// NewAgentRuntimeUsecase creates a new usecase backed by the given runtime.
func NewAgentRuntimeUsecase(runtime AgentRuntime) *AgentRuntimeUsecase {
	return &AgentRuntimeUsecase{runtime: runtime}
}

// ExecuteTask handles a TaskCommand by delegating to the runtime.
func (uc *AgentRuntimeUsecase) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if uc == nil || uc.runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return uc.runtime.ReceiveTask(ctx, cmd)
}

// Execute runs a prompt with the specified agent.
func (uc *AgentRuntimeUsecase) Execute(ctx context.Context, agent Agent, prompt string) (*taskv1.TaskResult, error) {
	if uc == nil || uc.runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if !uc.runtime.Supports(agent) {
		return nil, ErrAgentNotSupported
	}
	return uc.runtime.Execute(ctx, agent, prompt)
}

// VerifyDelegation verifies a delegated task result.
func (uc *AgentRuntimeUsecase) VerifyDelegation(ctx context.Context, taskID string, agent Agent, prompt string) (*taskv1.TaskResult, error) {
	if uc == nil || uc.runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	verifier, ok := uc.runtime.(DelegationVerifier)
	if !ok {
		return nil, ErrDelegationNotSupported
	}
	return verifier.VerifyDelegation(ctx, taskID, agent, prompt)
}
