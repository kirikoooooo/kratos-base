package service

import (
	"context"
	"testing"
)

func TestStartCLISSEServerRejectsNilStream(t *testing.T) {
	if _, err := StartCLISSEServer(context.Background(), "127.0.0.1:0", nil); err == nil {
		t.Fatal("StartCLISSEServer() error = nil, want nil stream rejection")
	}
}
