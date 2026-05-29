package biz

import (
	"context"
	"errors"

	taskv1 "kratos-demo/api/task/v1"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/google/wire"
)

var (
	ErrAgentRuntimeUnavailable = errors.New("agent runtime is not available")
	ErrAgentNotSupported       = errors.New("agent is not supported")
	ErrDelegationNotSupported  = errors.New("delegation verification is not supported")
)

var ProviderSet = wire.NewSet(NewAgentRuntimeUsecase)

type Agent string

const (
	AgentDefault  Agent = "default"
	AgentGeneric  Agent = "generic"
	AgentRouter   Agent = "router"
	AgentCoder    Agent = "coder"
	AgentReviewer Agent = "reviewer"
)

type AgentRuntime interface {
	actorpkg.Actor
	Name() string
	Supports(Agent) bool
	Execute(context.Context, Agent, string) (*taskv1.TaskResult, error)
	ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
	SendTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
}

type DelegationVerifier interface {
	VerifyDelegation(context.Context, string, Agent, string) (*taskv1.TaskResult, error)
}

type AgentRuntimeUsecase struct {
	runtime AgentRuntime
}

func NewAgentRuntimeUsecase(runtime AgentRuntime) *AgentRuntimeUsecase {
	return &AgentRuntimeUsecase{runtime: runtime}
}

func (uc *AgentRuntimeUsecase) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if uc == nil || uc.runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return uc.runtime.ReceiveTask(ctx, cmd)
}

func (uc *AgentRuntimeUsecase) Execute(ctx context.Context, agent Agent, prompt string) (*taskv1.TaskResult, error) {
	if uc == nil || uc.runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if !uc.runtime.Supports(agent) {
		return nil, ErrAgentNotSupported
	}
	return uc.runtime.Execute(ctx, agent, prompt)
}

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
