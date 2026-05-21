package biz

import (
	"time"

	taskv1 "kratos-demo/api/task/v1"
)

type DelegationTraceStore interface {
	StartTask(taskID string, agent TaskAgent, status TaskStatus)
	UpdateTask(taskID string, status TaskStatus, result *taskv1.TaskResult, err error)
	AppendEvent(event DelegationEvent)
	UpdatePlan(taskID string, steps []PlanStep)
	ListSessions(limit int) []DelegationSession
	UpdateContextUsage(taskID string, usage ContextUsageSnapshot, compress *ContextCompressResult)
}

type DelegationTraceSubscriber interface {
	Subscribe() (<-chan []DelegationSession, func())
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
}

type PlanStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

type DelegationSession struct {
	TaskID              string            `json:"task_id"`
	RootAgent           string            `json:"root_agent"`
	ConversationPreview string            `json:"conversation_preview,omitempty"`
	Status              string            `json:"status"`
	ResultSummary string            `json:"result_summary,omitempty"`
	ResultOutput  string            `json:"result_output,omitempty"`
	Error         string            `json:"error,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Plan          []PlanStep          `json:"plan,omitempty"`
	Events        []DelegationEvent   `json:"events"`
	FileChanges   []SessionFileChange  `json:"file_changes,omitempty"`
	ContextUsage  ContextUsageSnapshot `json:"context_usage,omitempty"`
}
