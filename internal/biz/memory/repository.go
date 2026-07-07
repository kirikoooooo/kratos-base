package memory

import (
	"context"

	bizagent "kratos-demo/internal/biz/agent"
	bizconversation "kratos-demo/internal/biz/conversation"
)

// AgentMemoryStore is the persistence interface for agent memory.
// Implementations live in the data layer.
type AgentMemoryStore interface {
	LoadUser(ctx context.Context, userID string) (*UserAgentMemory, error)
	SaveUser(ctx context.Context, memory *UserAgentMemory) error
	LoadSession(ctx context.Context, sessionID string) (*SessionAgentMemory, error)
	SaveSession(ctx context.Context, memory *SessionAgentMemory) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, memory *SessionConversation) error
	AppendSessionError(ctx context.Context, record SessionErrorRecord) error
	ListSessionErrors(ctx context.Context, sessionID string, limit int) ([]SessionErrorRecord, error)
}

// AgentMemory is the domain-level interface for agent memory operations.
// It adds business logic and orchestration on top of AgentMemoryStore.
type AgentMemory interface {
	UserID() string
	StartConversation(ctx context.Context, sessionID string, agent bizagent.AgentKind, initialPrompt string) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, conv *SessionConversation) error
	ConversationContextStats(ctx context.Context, sessionID string, turns []bizconversation.ConversationTurn) bizconversation.CompressStats
	PrepareTurnsForLLM(ctx context.Context, sessionID string, turns []bizconversation.ConversationTurn) []bizconversation.ConversationTurn
	PrepareTurnsForLLMWithMeta(ctx context.Context, sessionID string, turns []bizconversation.ConversationTurn) ([]bizconversation.ConversationTurn, bizconversation.PrepareMeta)
	ConversationContextUsage(ctx context.Context, sessionID string, turns []bizconversation.ConversationTurn) bizconversation.UsageSnapshot
	ConversationPreview(ctx context.Context, sessionID string) string
	PrepareForTask(ctx context.Context, sessionID string, agent bizagent.AgentKind) error
	RenderPromptContext(ctx context.Context, sessionID string) string
	RecordSessionError(ctx context.Context, record SessionErrorRecord) error
	RecordSessionToolUsage(ctx context.Context, sessionID, toolName string) error
	UpsertUserMemory(ctx context.Context, memory *UserAgentMemory) error
}
