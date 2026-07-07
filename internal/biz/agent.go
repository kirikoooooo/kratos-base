package biz

import (
	bizagent "kratos-demo/internal/biz/agent"

	"github.com/google/wire"
)

// Re-export from biz/agent sub-package for backward compatibility.
type (
	Agent                = bizagent.Agent
	AgentRuntime         = bizagent.AgentRuntime
	DelegationVerifier   = bizagent.DelegationVerifier
	AgentRuntimeUsecase  = bizagent.AgentRuntimeUsecase
)

var (
	AgentDefault  = bizagent.AgentDefault
	AgentGeneric  = bizagent.AgentGeneric
	AgentRouter   = bizagent.AgentRouter
	AgentCoder    = bizagent.AgentCoder
	AgentReviewer = bizagent.AgentReviewer

	ErrAgentRuntimeUnavailable = bizagent.ErrAgentRuntimeUnavailable
	ErrAgentNotSupported       = bizagent.ErrAgentNotSupported
	ErrDelegationNotSupported  = bizagent.ErrDelegationNotSupported

	NewAgentRuntimeUsecase = bizagent.NewAgentRuntimeUsecase
)

var ProviderSet = wire.NewSet(bizagent.ProviderSet)
