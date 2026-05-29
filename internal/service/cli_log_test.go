package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	datatrace "kratos-demo/internal/data/trace"
)

func TestNewCLIProcessLogWritesEvents(t *testing.T) {
	t.Chdir(t.TempDir())

	log, err := NewCLIProcessLog("cli-test")
	if err != nil {
		t.Fatalf("NewCLIProcessLog() error = %v", err)
	}
	defer log.Close()

	log.LogTurnStart("cli-test", "hello")
	log.LogEvent(datatrace.DelegationEvent{
		Stage:    "tool_read_file",
		ToolName: "read_file",
		Summary:  "README.md",
		Agent:    "router",
	})
	log.LogTurnDone("cli-test", "ok", "done", nil)

	raw, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{"session_start", "turn_start", "tool_read_file", "turn_done"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in log:\n%s", want, text)
		}
	}
	if !strings.HasPrefix(log.Path(), filepath.Join(".myagent", "log")) {
		if !strings.Contains(log.Path(), filepath.Join(".myagent", "log")) {
			t.Fatalf("unexpected log path: %s", log.Path())
		}
	}
}

func TestFormatCLIEventLogLineSkipsEmptyStage(t *testing.T) {
	if line := formatCLIEventLogLine(datatrace.DelegationEvent{}); line != "" {
		t.Fatalf("expected empty line, got %q", line)
	}
}
