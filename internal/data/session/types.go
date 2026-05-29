package session

import "time"

// Session 表示一次对话/任务会话；文件变更等会话内状态挂在 Session 上。
type Session interface {
	ID() string
	RecordFileChange(path, toolName, operation, summary, before, after string)
	FileChanges() []SessionFileChange
}

// SessionStore 按 sessionID 打开或创建 Session。
type SessionStore interface {
	Open(sessionID string) Session
}

// SessionFileChange 会话内单个文件的累积变更视图（baseline → current）。
type SessionFileChange struct {
	Path        string          `json:"path"`
	Status      string          `json:"status"` // created|modified|deleted|unchanged
	Baseline    string          `json:"baseline,omitempty"`
	Current     string          `json:"current,omitempty"`
	UnifiedDiff string          `json:"unified_diff,omitempty"`
	Operations  []SessionFileOp `json:"operations,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// SessionFileOp 单次工具导致的文件变更步骤。
type SessionFileOp struct {
	Time      time.Time `json:"time"`
	Tool      string    `json:"tool"`
	Operation string    `json:"operation,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}
