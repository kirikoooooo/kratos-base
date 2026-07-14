package agent_runtime

import (
	"context"

	taskv1 "kratos-demo/api/task/v1"
	errconst "kratos-demo/internal/consts/error"
	actorpkg "kratos-demo/third_party/actor"
)

var (
	ErrAgentRuntimeUnavailable = errconst.ErrAgentRuntimeUnavailable
	ErrAgentNotSupported       = errconst.ErrAgentNotSupported
	ErrDelegationNotSupported  = errconst.ErrDelegationNotSupported
)

// AgentRuntime is the core abstraction for any agent runtime implementation.
type AgentRuntime interface {
	actorpkg.Actor
	Name() string
	Supports(AgentKind) bool
	Execute(context.Context, AgentKind, string) (*taskv1.TaskResult, error)
	ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
	SendTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
}

// DelegationVerifier is an optional capability for runtimes that support
// delegation verification.
type DelegationVerifier interface {
	VerifyDelegation(context.Context, string, AgentKind, string) (*taskv1.TaskResult, error)
}
