package memory

import (
	"context"
	"strings"
	"time"

	"kratos-demo/internal/biz"
	agentcontext "kratos-demo/internal/data/agent/context"
	datatrace "kratos-demo/internal/data/trace"
)

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

type AgentMemory interface {
	UserID() string
	StartConversation(ctx context.Context, sessionID string, agent biz.Agent, initialPrompt string) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, conv *SessionConversation) error
	ConversationContextStats(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) agentcontext.CompressStats
	PrepareTurnsForLLM(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) []agentcontext.ConversationTurn
	PrepareTurnsForLLMWithMeta(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) ([]agentcontext.ConversationTurn, agentcontext.PrepareMeta)
	ConversationContextUsage(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) datatrace.ContextUsageSnapshot
	ConversationPreview(ctx context.Context, sessionID string) string
	PrepareForTask(ctx context.Context, sessionID string, agent biz.Agent) error
	RenderPromptContext(ctx context.Context, sessionID string) string
	RecordSessionError(ctx context.Context, record SessionErrorRecord) error
	RecordSessionToolUsage(ctx context.Context, sessionID, toolName string) error
	UpsertUserMemory(ctx context.Context, memory *UserAgentMemory) error
}

type SessionConversation struct {
	SessionID string                        `json:"session_id"`
	Agent     string                        `json:"agent,omitempty"`
	Turns     []agentcontext.ConversationTurn `json:"turns,omitempty"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

type CommandPolicy struct {
	Situation string   `json:"situation"`
	Commands  []string `json:"commands,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

type ToolHint struct {
	Name        string `json:"name"`
	WhenToUse   string `json:"when_to_use"`
	Constraints string `json:"constraints,omitempty"`
}

type SkillHint struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	WhenToUse   string `json:"when_to_use"`
	Description string `json:"description,omitempty"`
}

type PromptAdjustment struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type UserAgentMemory struct {
	UserID          string             `json:"user_id"`
	WorkspaceRoot   string             `json:"workspace_root,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	SkillHints      []SkillHint        `json:"skill_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type SessionAgentMemory struct {
	SessionID       string             `json:"session_id"`
	Agent           string             `json:"agent,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	ToolsUsed       []string           `json:"tools_used,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type AgentMemoryConfig struct {
	Dir                      string
	UserID                   string
	ContextCompressThreshold int
	KeepRecentTurns          int
	ToolOutputMaxChars       int
}

func (c AgentMemoryConfig) NormalizedUserID() string {
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return "default"
	}
	return userID
}

type SessionErrorRecord struct {
	Time      time.Time `json:"time"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	Stage     string    `json:"stage"`
	Tool      string    `json:"tool,omitempty"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
}
