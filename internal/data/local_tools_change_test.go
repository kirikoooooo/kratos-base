package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEditFileRecordsSessionChange(t *testing.T) {
	root := t.TempDir()
	trace := NewDelegationTraceStore()
	changes := NewSessionChangeStore(nil)
	tools, err := newLocalToolRuntime(trace, changes)
	if err != nil {
		t.Fatalf("newLocalToolRuntime() error = %v", err)
	}
	tools.root = root

	path := "note.txt"
	if err := os.WriteFile(filepath.Join(root, path), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	taskID := "session-edit-1"
	ctx := withTaskID(context.Background(), taskID)

	input, _ := json.Marshal(map[string]any{
		"path":        path,
		"operation":   "search_replace",
		"old_string":  "beta",
		"new_string":  "beta-updated",
	})
	_, err = tools.editFile(ctx, string(input))
	if err != nil {
		t.Fatalf("editFile() error = %v", err)
	}

	snapshot := changes.Snapshot(taskID)
	if len(snapshot) != 1 {
		t.Fatalf("changes len = %d, want 1", len(snapshot))
	}
	if snapshot[0].Path != path {
		t.Fatalf("path = %q, want %q", snapshot[0].Path, path)
	}
	if snapshot[0].Status != "modified" {
		t.Fatalf("status = %q, want modified", snapshot[0].Status)
	}
	if snapshot[0].UnifiedDiff == "" {
		t.Fatal("expected unified diff")
	}
}
