package memory

import (
	bizmemory "kratos-demo/internal/biz/memory"
)

// Re-export domain types from biz/memory for backward compatibility.
type (
	AgentMemoryStore    = bizmemory.AgentMemoryStore
	AgentMemory         = bizmemory.AgentMemory
	AgentMemoryConfig   = bizmemory.AgentMemoryConfig
	UserAgentMemory     = bizmemory.UserAgentMemory
	SessionAgentMemory  = bizmemory.SessionAgentMemory
	SessionConversation = bizmemory.SessionConversation
	SessionErrorRecord  = bizmemory.SessionErrorRecord
	CommandPolicy       = bizmemory.CommandPolicy
	ToolHint            = bizmemory.ToolHint
	SkillHint           = bizmemory.SkillHint
	PromptAdjustment    = bizmemory.PromptAdjustment
)

var (
	NewAgentMemoryStore        = NewFileAgentMemoryStore
	NewAgentMemoryConfig       = NewFileAgentMemoryConfig
	NewAgentMemoryUsecase      = NewFileAgentMemoryUsecase
	BootstrapUserMemoryIfEmpty = BootstrapFileUserMemoryIfEmpty
)
