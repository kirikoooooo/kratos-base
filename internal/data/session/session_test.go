package session

import (
	"os"
	"path/filepath"
	"testing"

	"kratos-demo/internal/conf"
)

func TestSessionRecordsAndSnapshotsFileChanges(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(&conf.Data{
		AgentMemory: &conf.Data_AgentMemory{Dir: dir},
	})
	taskID := "session-test-edit"
	sess := store.Open(taskID)
	sess.RecordFileChange("foo.txt", "edit_file", "search_replace", "replace greeting", "hello\n", "hello world\n")
	sess.RecordFileChange("foo.txt", "edit_file", "append", "append footer", "hello world\n", "hello world\nbye\n")

	changes := sess.FileChanges()
	if len(changes) != 1 {
		t.Fatalf("file changes = %d, want 1", len(changes))
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
	if len(change.Operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(change.Operations))
	}
	if change.UnifiedDiff == "" {
		t.Fatal("expected unified diff")
	}

	path := filepath.Join(dir, "changes", "session-test-edit.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("persisted change file missing: %v", err)
	}

	reloaded := NewSessionStore(&conf.Data{
		AgentMemory: &conf.Data_AgentMemory{Dir: dir},
	})
	reloadedChanges := reloaded.Open(taskID).FileChanges()
	if len(reloadedChanges) != 1 {
		t.Fatalf("reloaded file changes = %d, want 1", len(reloadedChanges))
	}
	if reloadedChanges[0].Current != "hello world\nbye\n" {
		t.Fatalf("reloaded current = %q", reloadedChanges[0].Current)
	}
}

func TestSessionCreateFile(t *testing.T) {
	store := NewSessionStore(nil)
	taskID := "session-test-create"
	store.Open(taskID).RecordFileChange("new.txt", "write_file", "create", "created new file", "", "content\n")

	changes := store.Open(taskID).FileChanges()
	if len(changes) != 1 || changes[0].Status != "created" {
		t.Fatalf("unexpected changes: %+v", changes)
	}
}
