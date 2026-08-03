package trace

import (
	"testing"

	"kratos-demo/internal/consts/public"
)

func TestSetTaskMetadataMergesValidValues(t *testing.T) {
	store := NewDelegationTraceStore()
	store.StartTask("task-1", public.AgentKindRouter, "running")
	store.SetTaskMetadata("task-1", map[string]any{
		"experiment":  "router-v1",
		"retry_count": 1,
		"bad key":     "ignored",
	})
	store.SetTaskMetadata("task-1", map[string]any{"experiment": "router-v2"})

	sessions := store.ListSessions(1)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	metadata := sessions[0].Metadata
	if got := metadata["experiment"]; got != "router-v2" {
		t.Fatalf("experiment = %v, want router-v2", got)
	}
	if got := metadata["retry_count"]; got != 1 {
		t.Fatalf("retry_count = %v, want 1", got)
	}
	if _, ok := metadata["bad key"]; ok {
		t.Fatal("invalid metadata key was stored")
	}
}
