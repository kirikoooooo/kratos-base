package biz

import (
	"strings"
	"testing"
	"time"
)

func TestFormatSessionErrorsForPrompt(t *testing.T) {
	records := []SessionErrorRecord{{
		Time:      time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),
		SessionID: "dashboard-router",
		Agent:     "router",
		Stage:     "tool_error",
		Tool:      "read_file",
		Message:   "file not found",
		Detail:    "path=internal/missing.go",
	}}
	got := FormatSessionErrorsForPrompt(records, ".kratos/agent", "dashboard-router")
	for _, want := range []string{
		"本会话近期错误",
		"dashboard-router.jsonl",
		"tool_error",
		"read_file",
		"file not found",
		"path=internal/missing.go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
