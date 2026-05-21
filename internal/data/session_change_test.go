package data

import (
	"os"
	"path/filepath"
	"testing"

	"kratos-demo/internal/conf"
)

func TestSessionChangeStoreRecordsAndSnapshots(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionChangeStore(&conf.Data{
		AgentMemory: &conf.Data_AgentMemory{Dir: dir},
	})

	taskID := "session-test-1"
	store.RecordChange(taskID, "foo.txt", "edit_file", "search_replace", "replace greeting", "hello\n", "hello world\n")
	store.RecordChange(taskID, "foo.txt", "edit_file", "append", "append footer", "hello world\n", "hello world\nbye\n")

	changes := store.Snapshot(taskID)
	if len(changes) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(changes))
	}
	change := changes[0]
	if change.Path != "foo.txt" {
		t.Fatalf("path = %q, want foo.txt", change.Path)
	}
	if change.Status != "modified" {
		t.Fatalf("status = %q, want modified", change.Status)
	}
	if change.Baseline != "hello\n" {
		t.Fatalf("baseline = %q", change.Baseline)
	}
	if change.Current != "hello world\nbye\n" {
		t.Fatalf("current = %q", change.Current)
	}
	if change.UnifiedDiff == "" {
		t.Fatal("expected unified diff")
	}
	if len(change.Operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(change.Operations))
	}

	persisted := filepath.Join(dir, "changes", safeFileName(taskID)+".json")
	if _, err := os.Stat(persisted); err != nil {
		t.Fatalf("expected persisted changes file: %v", err)
	}

	reloaded := NewSessionChangeStore(&conf.Data{
		AgentMemory: &conf.Data_AgentMemory{Dir: dir},
	})
	reloadedChanges := reloaded.Snapshot(taskID)
	if len(reloadedChanges) != 1 {
		t.Fatalf("reloaded snapshot len = %d, want 1", len(reloadedChanges))
	}
	if reloadedChanges[0].Path != "foo.txt" {
		t.Fatalf("reloaded path = %q", reloadedChanges[0].Path)
	}
}

func TestSessionChangeStoreCreateFile(t *testing.T) {
	store := NewSessionChangeStore(nil)
	taskID := "session-test-create"
	store.RecordChange(taskID, "new.txt", "write_file", "create", "created new file", "", "content\n")

	changes := store.Snapshot(taskID)
	if len(changes) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(changes))
	}
	if changes[0].Status != "created" {
		t.Fatalf("status = %q, want created", changes[0].Status)
	}
}
