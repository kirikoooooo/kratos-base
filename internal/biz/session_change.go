package biz

import "time"

// SessionChangeStore 记录单次对话/任务内的文件变更（非 git commit 粒度）。
type SessionChangeStore interface {
	RecordChange(taskID, path, toolName, operation, summary, before, after string)
	Snapshot(taskID string) []SessionFileChange
}

// SessionFileChange 会话内单个文件的累积变更视图（baseline → current）。
type SessionFileChange struct {
	Path         string          `json:"path"`
	Status       string          `json:"status"` // created|modified|deleted|unchanged
	Baseline     string          `json:"baseline,omitempty"`
	Current      string          `json:"current,omitempty"`
	UnifiedDiff  string          `json:"unified_diff,omitempty"`
	Operations   []SessionFileOp `json:"operations,omitempty"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// SessionFileOp 单次工具导致的文件变更步骤。
type SessionFileOp struct {
	Time      time.Time `json:"time"`
	Tool      string    `json:"tool"`
	Operation string    `json:"operation,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}
