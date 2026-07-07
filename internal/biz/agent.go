package biz

import (
	bizagent "kratos-demo/internal/biz/agent"
	bizagentruntime "kratos-demo/internal/biz/agent_runtime"

	"github.com/google/wire"
)

// Re-export Agent domain object from biz/agent.
type (
	Agent               = bizagent.Agent
	AgentRuntimeUsecase = bizagent.AgentRuntimeUsecase // = Agent
)

// Re-export AgentRuntime port types from biz/agent_runtime.
type (
	AgentKind          = bizagentruntime.AgentKind
	AgentRuntime       = bizagentruntime.AgentRuntime
	DelegationVerifier = bizagentruntime.DelegationVerifier
)

var (
	// AgentKind constants.
	AgentKindDefault  = bizagentruntime.AgentKindDefault
	AgentKindGeneric  = bizagentruntime.AgentKindGeneric
	AgentKindRouter   = bizagentruntime.AgentKindRouter
	AgentKindCoder    = bizagentruntime.AgentKindCoder
	AgentKindReviewer = bizagentruntime.AgentKindReviewer

	// Deprecated: use AgentKind constants.
	AgentDefault  = bizagentruntime.AgentKindDefault
	AgentGeneric  = bizagentruntime.AgentKindGeneric
	AgentRouter   = bizagentruntime.AgentKindRouter
	AgentCoder    = bizagentruntime.AgentKindCoder
	AgentReviewer = bizagentruntime.AgentKindReviewer

	// Error sentinels.
	ErrAgentRuntimeUnavailable = bizagentruntime.ErrAgentRuntimeUnavailable
	ErrAgentNotSupported       = bizagentruntime.ErrAgentNotSupported
	ErrDelegationNotSupported  = bizagentruntime.ErrDelegationNotSupported

	// Constructors.
	NewAgent               = bizagent.NewAgent
	NewAgentRuntimeUsecase = bizagent.NewAgentRuntimeUsecase
)

var ProviderSet = wire.NewSet(bizagent.ProviderSet, bizagentruntime.ProviderSet)
