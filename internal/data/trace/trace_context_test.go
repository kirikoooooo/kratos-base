package trace

import (
	"kratos-demo/internal/consts/public"
	"testing"
)

func TestUpdateContextUsageRecordsCompressCount(t *testing.T) {
	trace := NewDelegationTraceStore().(*memoryTraceStore)
	taskID := "ctx-task-1"
	trace.StartTask(taskID, public.AgentKindCoder, "running")

	usage := newContextUsageSnapshot(210000, 200000, true)
	compress := &ContextCompressResult{
		OriginalChars:   210000,
		CompressedChars: 45000,
		Compressed:      true,
		OmittedTurns:    12,
		TruncatedTools:  2,
	}
	trace.UpdateContextUsage(taskID, usage, compress)
	trace.AppendEvent(DelegationEvent{
		TaskID:  taskID,
		Stage:   "context_compress",
		Summary: formatContextCompressEventSummary(*compress),
	})

	sessions := trace.ListSessions(1)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	session := sessions[0]
	if session.ContextUsage.CompressCount != 1 {
		t.Fatalf("compress_count = %d, want 1", session.ContextUsage.CompressCount)
	}
	if session.ContextUsage.EstimatedChars != 210000 {
		t.Fatalf("estimated_chars = %d", session.ContextUsage.EstimatedChars)
	}
	found := false
	for _, event := range session.Events {
		if event.Stage == "context_compress" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected context_compress event")
	}
}
