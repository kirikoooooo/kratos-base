package biz

import (
	bizagent "kratos-demo/internal/biz/agent"
	bizagentruntime "kratos-demo/internal/biz/agent_runtime"
	errconst "kratos-demo/internal/consts/error"
	"kratos-demo/internal/consts/public"

	"github.com/google/wire"
)

// Re-export Agent domain object from biz/agent.
type (
	Agent               = bizagent.Agent
	AgentRuntimeUsecase = bizagent.AgentRuntimeUsecase // = Agent
)

// Re-export AgentRuntime port types from biz/agent_runtime + consts/public.
type (
	AgentKind          = public.AgentKind
	AgentRuntime       = bizagentruntime.AgentRuntime
	DelegationVerifier = bizagentruntime.DelegationVerifier
)

// Re-export sentinel errors from consts/error.
var (
	ErrAgentRuntimeUnavailable = errconst.ErrAgentRuntimeUnavailable
	ErrAgentNotSupported       = errconst.ErrAgentNotSupported
	ErrDelegationNotSupported  = errconst.ErrDelegationNotSupported
)

// Constructors.
var (
	NewAgent               = bizagent.NewAgent
	NewAgentRuntimeUsecase = bizagent.NewAgentRuntimeUsecase
)

var ProviderSet = wire.NewSet(bizagent.ProviderSet, bizagentruntime.ProviderSet)
