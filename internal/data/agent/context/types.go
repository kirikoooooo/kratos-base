package context

import datatrace "kratos-demo/internal/data/trace"

type ConversationRole string

const (	ConversationRoleHuman ConversationRole = "human"
	ConversationRoleAI    ConversationRole = "ai"
	ConversationRoleTool  ConversationRole = "tool"
)

type ConversationToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type ConversationTurn struct {
	Role       ConversationRole       `json:"role"`
	Content    string                 `json:"content,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolCalls  []ConversationToolCall `json:"tool_calls,omitempty"`
}

type PrepareMeta struct {
	Stats    CompressStats
	Compress *datatrace.ContextCompressResult
}

const (
	DefaultCompressThreshold = 200_000
	DefaultKeepRecentTurns   = 24
	DefaultToolOutputMaxChars = 8_000
)

type CompressConfig struct {
	Threshold          int
	KeepRecentTurns    int
	ToolOutputMaxChars int
}

type CompressStats struct {
	EstimatedChars int
	Threshold      int
	NeedsCompress  bool
}

func ConfigFromMemory(threshold, keepRecent, toolOutputMax int) CompressConfig {
	return CompressConfig{
		Threshold:          threshold,
		KeepRecentTurns:    keepRecent,
		ToolOutputMaxChars: toolOutputMax,
	}
}
