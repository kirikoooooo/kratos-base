package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCLIOutputStreamPublishesSSEEvent(t *testing.T) {
	stream := NewCLIOutputStream()
	mux := http.NewServeMux()
	RegisterCLIOutputStream(mux, stream)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/debug/cli/stream?session_id=cli-1", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rr, req); close(done) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rr.Body.String(), "event: ready") {
		time.Sleep(10 * time.Millisecond)
	}

	stream.Publish(CLIOutputEvent{SessionID: "cli-1", Type: "cli.progress", Text: "reading README"})
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rr.Body.String(), "event: cli.progress") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if body := rr.Body.String(); !strings.Contains(body, "event: cli.progress") || !strings.Contains(body, "reading README") {
		t.Fatalf("SSE response = %q", body)
	}
}
