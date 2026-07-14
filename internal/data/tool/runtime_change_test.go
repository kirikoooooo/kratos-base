package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	datasession "kratos-demo/internal/data/session"
	datatrace "kratos-demo/internal/data/trace"
)

func TestEditFileRecordsSessionChange(t *testing.T) {
	root := t.TempDir()
	trace := datatrace.NewDelegationTraceStore()
	sessions := datasession.NewSessionStore(nil)
	tools, err := NewRuntime(trace, sessions)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	tools.root = root

	path := "note.txt"
	if err := os.WriteFile(filepath.Join(root, path), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	taskID := "session-edit-1"
	ctx := agentctx.WithTaskID(context.Background(), taskID)

	input, _ := json.Marshal(map[string]any{
		"path":       path,
		"operation":  "search_replace",
		"old_string": "beta",
		"new_string": "beta-updated",
	})
	_, err = tools.editFile(ctx, string(input))
	if err != nil {
		t.Fatalf("editFile() error = %v", err)
	}

	changes := sessions.Open(taskID).FileChanges()
	if len(changes) != 1 {
		t.Fatalf("changes len = %d, want 1", len(changes))
	}
	if changes[0].Path != path {
		t.Fatalf("path = %q, want %q", changes[0].Path, path)
	}
	if changes[0].Status != "modified" {
		t.Fatalf("status = %q, want modified", changes[0].Status)
	}
	if changes[0].UnifiedDiff == "" {
		t.Fatal("expected unified diff")
	}
}
