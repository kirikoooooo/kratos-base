package agent

import agentmemory "kratos-demo/internal/data/memory"

type (
	AgentMemory        = agentmemory.AgentMemory
	AgentMemoryStore   = agentmemory.AgentMemoryStore
	AgentMemoryConfig  = agentmemory.AgentMemoryConfig
	SessionErrorRecord = agentmemory.SessionErrorRecord
)

var (
	NewAgentMemoryStore        = agentmemory.NewAgentMemoryStore
	NewAgentMemoryConfig       = agentmemory.NewAgentMemoryConfig
	NewAgentMemoryUsecase      = agentmemory.NewAgentMemoryUsecase
	BootstrapUserMemoryIfEmpty = agentmemory.BootstrapUserMemoryIfEmpty
)
