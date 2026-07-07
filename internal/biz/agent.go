package biz

import (
	bizagent "kratos-demo/internal/biz/agent"

	"github.com/google/wire"
)

// Re-export from biz/agent sub-package for backward compatibility.
type (
	// Agent is the domain-level orchestrator (aggregate root).
	Agent                = bizagent.Agent
	// AgentKind identifies a specific agent role (e.g. coder, reviewer).
	AgentKind            = bizagent.AgentKind
	// AgentRuntime is the core abstraction for agent runtime implementations.
	AgentRuntime         = bizagent.AgentRuntime
	// DelegationVerifier is an optional delegation verification capability.
	DelegationVerifier   = bizagent.DelegationVerifier
	// AgentRuntimeUsecase is a backward-compatible alias for Agent.
	AgentRuntimeUsecase  = bizagent.AgentRuntimeUsecase
)

var (
	// AgentKind constants — role identifiers for agent dispatch.
	AgentKindDefault  = bizagent.AgentKindDefault
	AgentKindGeneric  = bizagent.AgentKindGeneric
	AgentKindRouter   = bizagent.AgentKindRouter
	AgentKindCoder    = bizagent.AgentKindCoder
	AgentKindReviewer = bizagent.AgentKindReviewer

	// Deprecated: use AgentKind constants.
	AgentDefault  = bizagent.AgentKindDefault
	AgentGeneric  = bizagent.AgentKindGeneric
	AgentRouter   = bizagent.AgentKindRouter
	AgentCoder    = bizagent.AgentKindCoder
	AgentReviewer = bizagent.AgentKindReviewer

	// Error sentinels.
	ErrAgentRuntimeUnavailable = bizagent.ErrAgentRuntimeUnavailable
	ErrAgentNotSupported       = bizagent.ErrAgentNotSupported
	ErrDelegationNotSupported  = bizagent.ErrDelegationNotSupported

	// Constructors.
	NewAgent                = bizagent.NewAgent
	NewAgentRuntimeUsecase  = bizagent.NewAgentRuntimeUsecase
)

var ProviderSet = wire.NewSet(bizagent.ProviderSet)
