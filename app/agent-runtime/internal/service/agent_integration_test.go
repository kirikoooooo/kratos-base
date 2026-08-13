package service_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"
	"kratos-demo/app/agent-runtime/internal/service"
)

type dashboardMux struct{ *http.ServeMux }

func (m dashboardMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.ServeMux.HandleFunc(pattern, handler)
}

func TestDashboardStateIncludesToolTimeline(t *testing.T) {
	trace := datatrace.NewDelegationTraceStore()
	trace.StartTask("session-1", public.AgentKindDefault, string(datatasking.TaskStatusDone))
	trace.AppendEvent(datatrace.DelegationEvent{
		Time:          time.Now(),
		TaskID:        "session-1",
		Agent:         string(public.AgentKindDefault),
		Stage:         "tool_read_file",
		PromptPreview: "README.md",
		Summary:       "path: README.md",
	})
	trace.UpdateTask("session-1", string(datatasking.TaskStatusDone), &taskv1.TaskResult{
		Summary: "done",
		Output:  "first phase summary",
	}, nil)

	svc := service.NewDashboardService(trace, nil, nil, nil, &conf.Runtime{})
	router := dashboardMux{ServeMux: http.NewServeMux()}
	svc.Register(router)

	req := httptest.NewRequest("GET", "/debug/a2a/state", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("dashboard state status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"result_output":"first phase summary"`) {
		t.Fatalf("dashboard state missing result output: %s", body)
	}
	if !strings.Contains(body, `"stage":"tool_read_file"`) {
		t.Fatalf("dashboard state missing tool event: %s", body)
	}
}
