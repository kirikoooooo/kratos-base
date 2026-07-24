package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	"kratos-demo/internal/data"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"
	actorpkg "kratos-demo/third_party/actor"
)

type dashboardTestMux struct {
	*http.ServeMux
}

func (m dashboardTestMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.ServeMux.HandleFunc(pattern, handler)
}

func TestDashboardStreamReturnsStateEvent(t *testing.T) {
	trace := data.NewDelegationTraceStore()
	trace.StartTask("stream-1", public.AgentKindDefault, string(datatasking.TaskStatusRunning))
	trace.UpdatePlan("stream-1", []datatrace.PlanStep{{
		ID:     "understand",
		Title:  "理解任务",
		Status: "in_progress",
	}})
	trace.AppendEvent(datatrace.DelegationEvent{
		TaskID:   "stream-1",
		Agent:    string(public.AgentKindDefault),
		Stage:    "task_running",
		ToolName: "read_file",
	})
	trace.UpdateTask("stream-1", string(datatasking.TaskStatusDone), &taskv1.TaskResult{
		Summary: "done",
		Output:  "summary",
	}, nil)

	svc := NewDashboardService(trace, nil, nil, nil, &conf.Runtime{})
	mux := dashboardTestMux{ServeMux: http.NewServeMux()}
	svc.Register(mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/debug/a2a/stream", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rr, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rr.Body.String(), "event: state") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting stream handler to exit")
	}

	text := rr.Body.String()
	if !strings.Contains(text, "event: state") {
		t.Fatalf("stream response missing state event: %s", text)
	}
	if !strings.Contains(text, "\"plan\"") {
		t.Fatalf("stream response missing plan payload: %s", text)
	}
}

func TestDashboardTraceStreamReturnsEvent(t *testing.T) {
	trace := data.NewDelegationTraceStore()
	svc := NewDashboardService(trace, nil, nil, nil, &conf.Runtime{})
	mux := dashboardTestMux{ServeMux: http.NewServeMux()}
	svc.Register(mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/debug/a2a/events?task_id=trace-sse", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rr, req); close(done) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rr.Body.String(), "event: ready") {
		time.Sleep(10 * time.Millisecond)
	}

	trace.AppendEvent(datatrace.DelegationEvent{TaskID: "trace-sse", Stage: "tool_read_file"})
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rr.Body.String(), "event: trace.event") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if body := rr.Body.String(); !strings.Contains(body, "event: trace.event") || !strings.Contains(body, "tool_read_file") {
		t.Fatalf("SSE response = %q", body)
	}
}

func TestRunConversationAppendsFinalAnswerEvent(t *testing.T) {
	trace := data.NewDelegationTraceStore()
	runtime := fakeDashboardRuntime{
		result: &taskv1.TaskResult{
			Summary: "done",
			Output:  "final answer content",
		},
	}
	svc := NewDashboardService(trace, biz.NewAgentRuntimeUsecase(runtime), nil, nil, &conf.Runtime{})
	svc.markAgentStarted(public.AgentKindDefault)

	result, err := svc.runConversation(context.Background(), "session-final", public.AgentKindDefault, "summarize README")
	if err != nil {
		t.Fatalf("runConversation() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}

	sessions := trace.ListSessions(1)
	if len(sessions) != 1 {
		t.Fatalf("trace sessions = %d, want 1", len(sessions))
	}

	found := false
	for _, event := range sessions[0].Events {
		if event.Stage != "final_answer" {
			continue
		}
		found = true
		if event.Summary != "done" {
			t.Fatalf("final_answer summary = %q, want done", event.Summary)
		}
		if event.ToolOutput != "final answer content" {
			t.Fatalf("final_answer output = %q, want final answer content", event.ToolOutput)
		}
	}
	if !found {
		t.Fatalf("expected final_answer event, got events: %+v", sessions[0].Events)
	}
}

type fakeDashboardRuntime struct {
	result *taskv1.TaskResult
	err    error
}

func (f fakeDashboardRuntime) PID() actorpkg.PID           { return actorpkg.NewPID(1, "fake-dashboard-runtime") }
func (f fakeDashboardRuntime) Process(_ *actorpkg.Message) {}
func (f fakeDashboardRuntime) OnStop()                     {}
func (f fakeDashboardRuntime) Name() string                { return "fake-dashboard-runtime" }
func (f fakeDashboardRuntime) Supports(agent public.AgentKind) bool {
	switch agent {
	case public.AgentKindDefault, public.AgentKindRouter, public.AgentKindCoder, public.AgentKindReviewer:
		return true
	default:
		return false
	}
}
func (f fakeDashboardRuntime) Execute(_ context.Context, _ public.AgentKind, _ string) (*taskv1.TaskResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, errors.New("result is nil")
	}
	return f.result, nil
}
func (f fakeDashboardRuntime) ReceiveTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return f.Execute(ctx, public.AgentKind(cmd.GetAgent()), cmd.GetPrompt())
}
func (f fakeDashboardRuntime) SendTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return f.ReceiveTask(ctx, cmd)
}
