package agent

import (
	"context"
	"errors"

	taskv1 "kratos-demo/api/task/v1"
	actorpkg "kratos-demo/third_party/actor"
)

var (
	ErrAgentRuntimeUnavailable = errors.New("agent runtime is not available")
	ErrAgentNotSupported       = errors.New("agent is not supported")
	ErrDelegationNotSupported  = errors.New("delegation verification is not supported")
)

// AgentRuntime is the core abstraction for any agent runtime implementation.
type AgentRuntime interface {
	actorpkg.Actor
	Name() string
	Supports(Agent) bool
	Execute(context.Context, Agent, string) (*taskv1.TaskResult, error)
	ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
	SendTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
}

// DelegationVerifier is an optional capability for runtimes that support
// delegation verification.
type DelegationVerifier interface {
	VerifyDelegation(context.Context, string, Agent, string) (*taskv1.TaskResult, error)
}
