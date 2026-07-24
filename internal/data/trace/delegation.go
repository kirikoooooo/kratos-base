package trace

import (
	"context"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/consts/public"
	datasession "kratos-demo/internal/data/session"
)

type DelegationTraceStore interface {
	StartTask(taskID string, agent public.AgentKind, status string)
	UpdateTask(taskID string, status string, result *taskv1.TaskResult, err error)
	AppendEvent(event DelegationEvent)
	UpdatePlan(taskID string, steps []PlanStep)
	ListSessions(limit int) []DelegationSession
	UpdateContextUsage(taskID string, usage ContextUsageSnapshot, compress *ContextCompressResult)
}

// DelegationTraceObserver mirrors lifecycle data to an optional external backend.
// Implementations must not let transport failures affect task execution.
type DelegationTraceObserver interface {
	StartTask(taskID string, agent public.AgentKind, status string)
	UpdateTask(taskID string, status string, result *taskv1.TaskResult, err error)
	AppendEvent(event DelegationEvent)
	UpdatePlan(taskID string, steps []PlanStep)
	UpdateContextUsage(taskID string, usage ContextUsageSnapshot, compress *ContextCompressResult)
	Shutdown(ctx context.Context) error
}

type DelegationTraceSubscriber interface {
	Subscribe() (<-chan []DelegationSession, func())
}

// DelegationEventSubscriber publishes each trace change without requiring
// consumers to diff full session snapshots.
type DelegationEventSubscriber interface {
	SubscribeEvents() (<-chan TraceStreamEvent, func())
}

type TraceStreamEvent struct {
	Type      string             `json:"type"`
	TaskID    string             `json:"task_id"`
	Event     DelegationEvent    `json:"event,omitempty"`
	Session   *DelegationSession `json:"session,omitempty"`
	Published time.Time          `json:"published"`
}

type DelegationEvent struct {
	Time          time.Time `json:"time"`
	TaskID        string    `json:"task_id"`
	Agent         string    `json:"agent"`
	Stage         string    `json:"stage"`
	Mode          string    `json:"mode,omitempty"`
	Target        string    `json:"target,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	Error         string    `json:"error,omitempty"`
	PromptPreview string    `json:"prompt_preview,omitempty"`
	DurationMS    int64     `json:"duration_ms,omitempty"`
	ToolName      string    `json:"tool_name,omitempty"`
	ToolInput     string    `json:"tool_input,omitempty"`
	ToolOutput    string    `json:"tool_output,omitempty"`
	ExitCode      int       `json:"exit_code,omitempty"`
	Model         string    `json:"model,omitempty"`
	InputTokens   int       `json:"input_tokens,omitempty"`
	OutputTokens  int       `json:"output_tokens,omitempty"`
}

type PlanStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

type ContextUsageSnapshot struct {
	EstimatedChars      int       `json:"estimated_chars"`
	Threshold           int       `json:"threshold"`
	UsagePercent        float64   `json:"usage_percent"`
	NeedsCompress       bool      `json:"needs_compress"`
	CompressCount       int       `json:"compress_count"`
	LastOriginalChars   int       `json:"last_original_chars,omitempty"`
	LastCompressedChars int       `json:"last_compressed_chars,omitempty"`
	LastOmittedTurns    int       `json:"last_omitted_turns,omitempty"`
	LastTruncatedTools  int       `json:"last_truncated_tools,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ContextCompressResult struct {
	OriginalChars   int
	CompressedChars int
	Compressed      bool
	OmittedTurns    int
	TruncatedTools  int
}

type DelegationSession struct {
	TaskID              string                          `json:"task_id"`
	RootAgent           string                          `json:"root_agent"`
	ConversationPreview string                          `json:"conversation_preview,omitempty"`
	Status              string                          `json:"status"`
	ResultSummary       string                          `json:"result_summary,omitempty"`
	ResultOutput        string                          `json:"result_output,omitempty"`
	Error               string                          `json:"error,omitempty"`
	CreatedAt           time.Time                       `json:"created_at"`
	UpdatedAt           time.Time                       `json:"updated_at"`
	Plan                []PlanStep                      `json:"plan,omitempty"`
	Events              []DelegationEvent               `json:"events"`
	FileChanges         []datasession.SessionFileChange `json:"file_changes,omitempty"`
	ContextUsage        ContextUsageSnapshot            `json:"context_usage,omitempty"`
}
