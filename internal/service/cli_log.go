package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/data/common"
	datatrace "kratos-demo/internal/data/trace"
)

type CLIProcessLog struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func NewCLIProcessLog(sessionID string) (*CLIProcessLog, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("cli-%d", time.Now().Unix())
	}

	dir, err := cliLogDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	path := filepath.Join(dir, common.SafeFileName(sessionID)+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	l := &CLIProcessLog{file: f, path: path}
	l.writeLine("session_start session_id=%s log=%s", sessionID, path)
	return l, nil
}

func cliLogDir() (string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(workspace, common.DefaultMemoryDir, "log")
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(workspace, dir)
	}
	return dir, nil
}

func (l *CLIProcessLog) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *CLIProcessLog) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *CLIProcessLog) Write(p []byte) (int, error) {
	if l == nil || l.file == nil {
		return len(p), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Write(p)
}

func (l *CLIProcessLog) writeLine(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format(time.RFC3339)
	fmt.Fprintf(l.file, "%s %s\n", ts, fmt.Sprintf(format, args...))
}

func (l *CLIProcessLog) LogTurnStart(sessionID, prompt string) {
	if l == nil {
		return
	}
	l.writeLine("turn_start session_id=%s prompt=%q", sessionID, strings.TrimSpace(prompt))
}

func (l *CLIProcessLog) LogTurnDone(sessionID, summary, output string, err error) {
	if l == nil {
		return
	}
	if err != nil {
		l.writeLine("turn_failed session_id=%s error=%q", sessionID, err.Error())
		return
	}
	l.writeLine("turn_done session_id=%s summary=%q output_chars=%d", sessionID, strings.TrimSpace(summary), len(strings.TrimSpace(output)))
}

func (l *CLIProcessLog) LogEvent(event datatrace.DelegationEvent) {
	if l == nil {
		return
	}
	line := formatCLIEventLogLine(event)
	if line == "" {
		return
	}
	l.writeLine("%s", line)
}

func formatCLIEventLogLine(event datatrace.DelegationEvent) string {
	stage := strings.TrimSpace(event.Stage)
	if stage == "" {
		return ""
	}
	parts := []string{
		"event",
		"stage=" + stage,
		"agent=" + strings.TrimSpace(event.Agent),
		"task_id=" + strings.TrimSpace(event.TaskID),
	}
	if v := strings.TrimSpace(event.Mode); v != "" {
		parts = append(parts, "mode="+v)
	}
	if v := strings.TrimSpace(event.ToolName); v != "" {
		parts = append(parts, "tool="+v)
	}
	if v := strings.TrimSpace(event.Summary); v != "" {
		parts = append(parts, "summary="+quoteLogField(v))
	}
	if v := strings.TrimSpace(event.Error); v != "" {
		parts = append(parts, "error="+quoteLogField(v))
	}
	if v := strings.TrimSpace(event.ToolInput); v != "" {
		parts = append(parts, "tool_input="+quoteLogField(truncateLogField(v, 500)))
	}
	if v := strings.TrimSpace(event.ToolOutput); v != "" {
		parts = append(parts, "tool_output="+quoteLogField(truncateLogField(v, 500)))
	}
	if event.DurationMS > 0 {
		parts = append(parts, fmt.Sprintf("duration_ms=%d", event.DurationMS))
	}
	if event.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", event.ExitCode))
	}
	return strings.Join(parts, " ")
}

func quoteLogField(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if strings.ContainsAny(value, " \t\n\"") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

func truncateLogField(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

var _ io.Writer = (*CLIProcessLog)(nil)
