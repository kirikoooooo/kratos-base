package conversation

// ConversationRole identifies who produced a conversation turn.
type ConversationRole string

const (
	ConversationRoleHuman ConversationRole = "human"
	ConversationRoleAI    ConversationRole = "ai"
	ConversationRoleTool  ConversationRole = "tool"
)

// ConversationToolCall represents a tool invocation requested by the LLM.
type ConversationToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// ConversationTurn is one turn in a conversation (human prompt, AI response, or tool result).
type ConversationTurn struct {
	Role       ConversationRole       `json:"role"`
	Content    string                 `json:"content,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolCalls  []ConversationToolCall `json:"tool_calls,omitempty"`
}

// CompressConfig controls context compression behaviour.
type CompressConfig struct {
	Threshold          int
	KeepRecentTurns    int
	ToolOutputMaxChars int
}

// ConfigFromMemory builds a CompressConfig from raw memory config values.
func ConfigFromMemory(threshold, keepRecent, toolOutputMax int) CompressConfig {
	return CompressConfig{
		Threshold:          threshold,
		KeepRecentTurns:    keepRecent,
		ToolOutputMaxChars: toolOutputMax,
	}
}

// CompressStats summarises the current compression state of a conversation.
type CompressStats struct {
	EstimatedChars int
	Threshold      int
	NeedsCompress  bool
}

// CompressResult records the outcome of a compression pass.
type CompressResult struct {
	OriginalChars   int
	CompressedChars int
	Compressed      bool
	OmittedTurns    int
	TruncatedTools  int
}

// PrepareMeta bundles compression metadata produced when preparing turns for the LLM.
type PrepareMeta struct {
	Stats    CompressStats
	Compress *CompressResult
}

// UsageSnapshot records the context window utilisation at a point in time.
type UsageSnapshot struct {
	EstimatedChars      int     `json:"estimated_chars"`
	Threshold           int     `json:"threshold"`
	UsagePercent        float64 `json:"usage_percent"`
	NeedsCompress       bool    `json:"needs_compress"`
	CompressCount       int     `json:"compress_count"`
	LastOriginalChars   int     `json:"last_original_chars,omitempty"`
	LastCompressedChars int     `json:"last_compressed_chars,omitempty"`
	LastOmittedTurns    int     `json:"last_omitted_turns,omitempty"`
	LastTruncatedTools  int     `json:"last_truncated_tools,omitempty"`
}

const (
	DefaultCompressThreshold  = 200_000
	DefaultKeepRecentTurns    = 24
	DefaultToolOutputMaxChars = 8_000
)
