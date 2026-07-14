package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	agentcontext "kratos-demo/internal/data/agent_runtime/context"
)

func TestFileAgentMemoryStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := &fileAgentMemoryStore{baseDir: dir, userID: "default"}
	if err := store.ensureLayout(); err != nil {
		t.Fatalf("ensureLayout: %v", err)
	}

	ctx := context.Background()
	user := &UserAgentMemory{
		UserID: "default",
		ToolHints: []ToolHint{{
			Name:      "read_file",
			WhenToUse: "read docs first",
		}},
	}
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	loadedUser, err := store.LoadUser(ctx, "default")
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if len(loadedUser.ToolHints) != 1 || loadedUser.ToolHints[0].Name != "read_file" {
		t.Fatalf("unexpected user memory: %+v", loadedUser)
	}

	session := &SessionAgentMemory{
		SessionID: "task-9",
		Agent:     "default",
		ToolsUsed: []string{"read_file"},
	}
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	loadedSession, err := store.LoadSession(ctx, "task-9")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loadedSession.SessionID != "task-9" || len(loadedSession.ToolsUsed) != 1 {
		t.Fatalf("unexpected session memory: %+v", loadedSession)
	}

	userPath := filepath.Join(dir, "users", "default.json")
	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("user memory file missing: %v", err)
	}

	conv := &SessionConversation{
		SessionID: "task-9",
		Turns: []agentcontext.ConversationTurn{
			{Role: agentcontext.ConversationRoleHuman, Content: "hello"},
			{Role: agentcontext.ConversationRoleAI, Content: "world"},
		},
	}
	if err := store.SaveConversation(ctx, conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	loadedConv, err := store.LoadConversation(ctx, "task-9")
	if err != nil {
		t.Fatalf("LoadConversation: %v", err)
	}
	if len(loadedConv.Turns) != 2 {
		t.Fatalf("unexpected conversation: %+v", loadedConv)
	}

	rec := SessionErrorRecord{
		SessionID: "task-9",
		Agent:     "default",
		Stage:     "tool_error",
		Tool:      "read_file",
		Message:   "not found",
	}
	if err := store.AppendSessionError(ctx, rec); err != nil {
		t.Fatalf("AppendSessionError: %v", err)
	}
	if err := store.AppendSessionError(ctx, SessionErrorRecord{
		SessionID: "task-9",
		Stage:     "task_failed",
		Message:   "boom",
	}); err != nil {
		t.Fatalf("AppendSessionError second: %v", err)
	}
	listed, err := store.ListSessionErrors(ctx, "task-9", 1)
	if err != nil {
		t.Fatalf("ListSessionErrors: %v", err)
	}
	if len(listed) != 1 || listed[0].Stage != "task_failed" {
		t.Fatalf("unexpected listed errors: %+v", listed)
	}
	errPath := filepath.Join(dir, "errors", "task-9.jsonl")
	if _, err := os.Stat(errPath); err != nil {
		t.Fatalf("error log file missing: %v", err)
	}
}
