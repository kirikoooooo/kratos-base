package langfuse

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
)

func TestObserverExportsOfficialOTLPRequest(t *testing.T) {
	var authorization, path string
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		path = r.URL.Path
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("TEST_LANGFUSE_PUBLIC", "pk-test")
	t.Setenv("TEST_LANGFUSE_SECRET", "sk-test")
	observer, cleanup, err := New(&conf.Observability{Langfuse: &conf.Observability_Langfuse{
		Enabled:      true,
		Host:         server.URL,
		PublicKeyEnv: "TEST_LANGFUSE_PUBLIC",
		SecretKeyEnv: "TEST_LANGFUSE_SECRET",
	}}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	observer.StartTask("task-1", public.AgentKind("coder"), "running")
	observer.AppendEvent(datatrace.DelegationEvent{TaskID: "task-1", Stage: "tool_read_file", ToolName: "read_file", ToolInput: `{"path":"a.go"}`})
	observer.UpdateTask("task-1", "done", nil, nil)
	cleanup()
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive OTLP export")
	}
	if path != "/api/public/otel/v1/traces" {
		t.Fatalf("path = %q", path)
	}
	if authorization != "Basic cGstdGVzdDpzay10ZXN0" {
		t.Fatalf("authorization = %q", authorization)
	}
}

func TestLimit(t *testing.T) {
	got := limit(strings.Repeat("a", maxObservedValue+1))
	if len(got) != maxObservedValue+3 {
		t.Fatalf("unexpected size %d", len(got))
	}
}

func TestBaseURLEnvironmentOverridesConfig(t *testing.T) {
	t.Setenv("LANGFUSE_BASE_URL", "https://self-hosted.example")
	if got := strings.TrimRight(defaultString(os.Getenv("LANGFUSE_BASE_URL"), "https://cloud.langfuse.com"), "/"); got != "https://self-hosted.example" {
		t.Fatalf("base URL = %q", got)
	}
}
