package trace

import (
	"testing"
	"time"
)

func TestTraceStorePublishesAppendedEvent(t *testing.T) {
	store := NewDelegationTraceStore()
	subscriber, ok := store.(DelegationEventSubscriber)
	if !ok {
		t.Fatal("trace store does not implement DelegationEventSubscriber")
	}
	stream, cancel := subscriber.SubscribeEvents()
	defer cancel()

	store.AppendEvent(DelegationEvent{TaskID: "task-stream", Stage: "tool_read_file"})
	select {
	case event := <-stream:
		if event.Type != "trace.event" || event.TaskID != "task-stream" || event.Event.Stage != "tool_read_file" {
			t.Fatalf("stream event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for trace event")
	}
}
