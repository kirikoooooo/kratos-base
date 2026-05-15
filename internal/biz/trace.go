package biz

import (
	"time"

	taskv1 "kratos-demo/api/task/v1"
)

type DelegationTraceStore interface {
	StartTask(taskID string, agent TaskAgent, prompt string, status TaskStatus)
	UpdateTask(taskID string, status TaskStatus, result *taskv1.TaskResult, err error)
	AppendEvent(event DelegationEvent)
	ListSessions(limit int) []DelegationSession
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
}

type DelegationSession struct {
	TaskID        string            `json:"task_id"`
	RootAgent     string            `json:"root_agent"`
	Prompt        string            `json:"prompt"`
	Status        string            `json:"status"`
	ResultSummary string            `json:"result_summary,omitempty"`
	ResultOutput  string            `json:"result_output,omitempty"`
	Error         string            `json:"error,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Events        []DelegationEvent `json:"events"`
}
