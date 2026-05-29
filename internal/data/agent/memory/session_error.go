package memory
import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

)
const (
	maxSessionErrorMessage       = 2000
	maxSessionErrorDetail        = 4000
	defaultSessionErrorsInPrompt = 8
)

func truncateSessionErrorText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func formatSessionErrorsForPrompt(records []SessionErrorRecord, memoryDir, sessionID string) string {
	if len(records) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 本会话近期错误（供排查与修正，勿重复相同失败）\n")
	b.WriteString("完整日志文件: ")
	b.WriteString(sessionErrorsLogHint(memoryDir, sessionID))
	b.WriteString("\n")
	for i, rec := range records {
		if i > 0 {
			b.WriteString("\n")
		}
		ts := rec.Time.Format(time.RFC3339)
		if rec.Time.IsZero() {
			ts = "unknown"
		}
		line := fmt.Sprintf("- [%s] stage=%s", ts, strings.TrimSpace(rec.Stage))
		if agent := strings.TrimSpace(rec.Agent); agent != "" {
			line += " agent=" + agent
		}
		if tool := strings.TrimSpace(rec.Tool); tool != "" {
			line += " tool=" + tool
		}
		if msg := strings.TrimSpace(rec.Message); msg != "" {
			line += " | " + msg
		}
		b.WriteString(line)
		if detail := strings.TrimSpace(rec.Detail); detail != "" {
			b.WriteString("\n  detail: ")
			b.WriteString(detail)
		}
	}
	return strings.TrimSpace(b.String())
}

func sessionErrorsLogHint(memoryDir, sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "errors/<session>.jsonl"
	}
	name := sessionID
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	name = replacer.Replace(name)
	if name == "" {
		name = "unknown"
	}
	rel := filepath.Join("errors", name+".jsonl")
	if strings.TrimSpace(memoryDir) == "" {
		return rel
	}
	return filepath.Join(strings.TrimSpace(memoryDir), rel)
}
