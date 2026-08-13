package service

import (
	"context"
	"testing"

	memoryv1 "kratos-demo/api/memory/v1"
)

type fakePromptContextReader struct {
	userID    string
	sessionID string
}

func (f *fakePromptContextReader) PromptContext(_ context.Context, userID, sessionID string) string {
	f.userID = userID
	f.sessionID = sessionID
	return "persistent context"
}

func TestGetPromptContext(t *testing.T) {
	reader := new(fakePromptContextReader)
	service := NewMemoryService(reader)
	reply, err := service.GetPromptContext(context.Background(), &memoryv1.GetPromptContextRequest{UserId: "user-1", SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.GetContent() != "persistent context" || reader.userID != "user-1" || reader.sessionID != "session-1" {
		t.Fatalf("unexpected prompt context reply=%q reader=%+v", reply.GetContent(), reader)
	}
}
