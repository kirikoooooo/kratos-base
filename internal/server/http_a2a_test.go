package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	actorpkg "kratos-demo/third_party/actor"
)

func TestHTTPServerPublishesA2AAgentCard(t *testing.T) {
	srv := NewHTTPServer(&conf.Server{Http: &conf.Server_HTTP{}}, nil, nil, httpRuntime{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

type httpRuntime struct{}

func (httpRuntime) PID() actorpkg.PID              { return actorpkg.NewPID(1, "http-runtime") }
func (httpRuntime) Process(*actorpkg.Message)      {}
func (httpRuntime) OnStop()                        {}
func (httpRuntime) Name() string                   { return "stub" }
func (httpRuntime) Supports(public.AgentKind) bool { return true }
func (httpRuntime) Execute(context.Context, public.AgentKind, string) (*taskv1.TaskResult, error) {
	return nil, nil
}
func (httpRuntime) ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return nil, nil
}
func (httpRuntime) SendTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return nil, nil
}

var _ biz.AgentRuntime = httpRuntime{}
