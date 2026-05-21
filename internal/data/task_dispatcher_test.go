package data

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/service"
	actorpkg "kratos-demo/third_party/actor"
	toolcatalog "kratos-demo/third_party/tools"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	grpcserver "google.golang.org/grpc"
)

type fakeLLM struct{}

type dashboardMux struct {
	*http.ServeMux
}

func (m dashboardMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.ServeMux.HandleFunc(pattern, handler)
}

func (fakeLLM) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if text, ok := part.(llms.TextContent); ok {
				parts = append(parts, text.Text)
			}
		}
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: strings.Join(parts, "\n")}}}, nil
}

func (fakeLLM) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	return prompt, nil
}

type scriptedToolLoopLLM struct {
	callCount int
}

func (m *scriptedToolLoopLLM) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	m.callCount++
	switch m.callCount {
	case 1:
		if len(messages) != 2 {
			return nil, fmt.Errorf("first call message count = %d, want 2", len(messages))
		}
		if messages[0].Role != llms.ChatMessageTypeSystem {
			return nil, fmt.Errorf("first message role = %s, want system", messages[0].Role)
		}
		if messages[1].Role != llms.ChatMessageTypeHuman {
			return nil, fmt.Errorf("second message role = %s, want human", messages[1].Role)
		}
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				ToolCalls: []llms.ToolCall{{
					ID:   "call-readme",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "read_file",
						Arguments: `{"input":"README.md"}`,
					},
				}},
			}},
		}, nil
	case 2:
		if len(messages) != 4 {
			return nil, fmt.Errorf("second call message count = %d, want 4", len(messages))
		}
		if messages[2].Role != llms.ChatMessageTypeAI {
			return nil, fmt.Errorf("assistant tool-call role = %s, want ai", messages[2].Role)
		}
		if len(messages[2].Parts) != 1 {
			return nil, fmt.Errorf("assistant tool-call parts = %d, want 1", len(messages[2].Parts))
		}
		toolCall, ok := messages[2].Parts[0].(llms.ToolCall)
		if !ok {
			return nil, fmt.Errorf("assistant tool-call part type = %T, want llms.ToolCall", messages[2].Parts[0])
		}
		if toolCall.FunctionCall == nil || toolCall.FunctionCall.Name != "read_file" {
			return nil, fmt.Errorf("assistant tool-call function = %+v", toolCall.FunctionCall)
		}

		if messages[3].Role != llms.ChatMessageTypeTool {
			return nil, fmt.Errorf("tool response role = %s, want tool", messages[3].Role)
		}
		if len(messages[3].Parts) != 1 {
			return nil, fmt.Errorf("tool response parts = %d, want 1", len(messages[3].Parts))
		}
		toolResp, ok := messages[3].Parts[0].(llms.ToolCallResponse)
		if !ok {
			return nil, fmt.Errorf("tool response part type = %T, want llms.ToolCallResponse", messages[3].Parts[0])
		}
		if toolResp.ToolCallID != "call-readme" {
			return nil, fmt.Errorf("tool response id = %q, want call-readme", toolResp.ToolCallID)
		}
		if toolResp.Name != "read_file" {
			return nil, fmt.Errorf("tool response name = %q, want read_file", toolResp.Name)
		}
		if !strings.Contains(toolResp.Content, "README.md") {
			return nil, fmt.Errorf("tool response content = %q, want README.md", toolResp.Content)
		}
		for _, msg := range messages {
			if msg.Role == llms.ChatMessageTypeFunction {
				return nil, fmt.Errorf("unexpected legacy function role found in messages")
			}
		}

		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				Content: "已读取 README.md 并完成总结",
			}},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected GenerateContent call count: %d", m.callCount)
	}
}

func (m *scriptedToolLoopLLM) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	return prompt, nil
}

type selfCorrectingToolLoopLLM struct {
	callCount int
}

func (m *selfCorrectingToolLoopLLM) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	m.callCount++
	switch m.callCount {
	case 1:
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				ToolCalls: []llms.ToolCall{{
					ID:   "call-bad-path",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "read_file",
						Arguments: `{"input":"configs/app.yml"}`,
					},
				}},
			}},
		}, nil
	case 2:
		if len(messages) != 4 {
			return nil, fmt.Errorf("second call message count = %d, want 4", len(messages))
		}
		toolResp, ok := messages[3].Parts[0].(llms.ToolCallResponse)
		if !ok {
			return nil, fmt.Errorf("tool response part type = %T, want llms.ToolCallResponse", messages[3].Parts[0])
		}
		if !strings.Contains(toolResp.Content, "status: error") {
			return nil, fmt.Errorf("expected error observation, got %q", toolResp.Content)
		}
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				ToolCalls: []llms.ToolCall{{
					ID:   "call-good-path",
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      "read_file",
						Arguments: `{"input":"configs/config.yaml"}`,
					},
				}},
			}},
		}, nil
	case 3:
		if len(messages) != 6 {
			return nil, fmt.Errorf("third call message count = %d, want 6", len(messages))
		}
		toolResp, ok := messages[5].Parts[0].(llms.ToolCallResponse)
		if !ok {
			return nil, fmt.Errorf("tool response part type = %T, want llms.ToolCallResponse", messages[5].Parts[0])
		}
		if !strings.Contains(toolResp.Content, "path: configs/config.yaml") {
			return nil, fmt.Errorf("expected successful observation, got %q", toolResp.Content)
		}
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				Content: "已修正路径并成功读取配置文件",
			}},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected GenerateContent call count: %d", m.callCount)
	}
}

func (m *selfCorrectingToolLoopLLM) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	return prompt, nil
}

type alwaysFailingToolLoopLLM struct {
	callCount int
}

func (m *alwaysFailingToolLoopLLM) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	m.callCount++
	if m.callCount > maxToolCorrectionRounds {
		return nil, fmt.Errorf("GenerateContent called too many times: %d", m.callCount)
	}
	if m.callCount > 1 {
		last := messages[len(messages)-1]
		if last.Role != llms.ChatMessageTypeTool {
			return nil, fmt.Errorf("last message role = %s, want tool", last.Role)
		}
		toolResp, ok := last.Parts[0].(llms.ToolCallResponse)
		if !ok {
			return nil, fmt.Errorf("tool response part type = %T, want llms.ToolCallResponse", last.Parts[0])
		}
		if !strings.Contains(toolResp.Content, "status: error") {
			return nil, fmt.Errorf("expected error observation, got %q", toolResp.Content)
		}
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{
			ToolCalls: []llms.ToolCall{{
				ID:   fmt.Sprintf("call-fail-%d", m.callCount),
				Type: "function",
				FunctionCall: &llms.FunctionCall{
					Name:      "read_file",
					Arguments: `{"input":"configs/missing.yaml"}`,
				},
			}},
		}},
	}, nil
}

func (m *alwaysFailingToolLoopLLM) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	return prompt, nil
}

func withFakeRuntimeLLM(t *testing.T) {
	t.Helper()
	prev := newRuntimeLLM
	prevPurpose := newRuntimeLLMForPurpose
	newRuntimeLLM = func(_ *conf.AI, _ *log.Helper) (llms.Model, error) {
		return fakeLLM{}, nil
	}
	newRuntimeLLMForPurpose = func(_ *conf.AI, _ *log.Helper, _ string) (llms.Model, error) {
		return fakeLLM{}, nil
	}
	t.Cleanup(func() {
		newRuntimeLLM = prev
		newRuntimeLLMForPurpose = prevPurpose
	})
}

func TestAgentRuntimeSendTaskViaActor(t *testing.T) {
	withFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	trace := NewDelegationTraceStore()
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	_ = NewTaskDispatcher(NewTaskRepo(logger), runtime, trace, nil, logger)

	result, err := runtime.SendTask(context.Background(), &taskv1.TaskCommand{
		TaskID: "task-sync",
		Agent:  biz.TaskAgentCoder.String(),
		Prompt: "implement a minimal example",
	})

	if err != nil {
		t.Fatalf("expected actor send task success, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.Summary == "" {
		t.Fatal("expected non-empty task result summary")
	}
}

func TestTaskDispatcherDispatchViaActor(t *testing.T) {
	withFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	repo := NewTaskRepo(logger)
	trace := NewDelegationTraceStore()
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	dispatcher := NewTaskDispatcher(repo, runtime, trace, nil, logger)

	task := &biz.Task{
		ID:        "task-dispatch",
		Agent:     biz.TaskAgentReviewer,
		Prompt:    "review boundary conditions",
		Status:    biz.TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), task.ToCommand()); err != nil {
		t.Fatalf("dispatch task failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := repo.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("get task failed: %v", err)
		}
		if stored.Status == biz.TaskStatusDone {
			if stored.Result == nil {
				t.Fatal("expected task result after actor dispatch")
			}
			if stored.Result.Summary == "" {
				t.Fatal("expected non-empty task result summary after actor dispatch")
			}
			return
		}
		if stored.Status == biz.TaskStatusFailed {
			t.Fatalf("expected task done, got failed: %s", stored.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for actor-dispatched task to complete")
}

func TestDispatchSubTaskViaRemoteGRPC(t *testing.T) {
	withFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	trace := NewDelegationTraceStore()
	remoteRuntime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	remoteService := service.NewAgentRuntimeService(remoteRuntime)

	grpcSrv := grpcserver.NewServer()
	taskv1.RegisterAgentRuntimeServiceServer(grpcSrv, remoteService)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen grpc server failed: %v", err)
	}
	defer listener.Close()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- grpcSrv.Serve(listener)
	}()
	defer func() {
		grpcSrv.Stop()
		select {
		case err := <-serveDone:
			if err != nil {
				t.Fatalf("grpc server stopped with error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting grpc server to stop")
		}
	}()

	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{
		Remotes: []*conf.Runtime_RemoteAgent{{
			Agent:   biz.TaskAgentCoder.String(),
			Target:  listener.Addr().String(),
			Timeout: 3,
		}},
	}, trace, nil, nil, logger)

	result, err := runtime.(*langChainAgentRuntime).dispatchSubTask(context.Background(), biz.TaskAgentCoder, "remote implement a minimal endpoint")
	if err != nil {
		t.Fatalf("dispatch remote grpc sub task failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil remote task result")
	}
	if result.GetSummary() == "" {
		t.Fatal("expected non-empty remote task result summary")
	}
	if result.GetOutput() == "" {
		t.Fatal("expected non-empty remote task result output")
	}
	if result.GetSummary() != "coder agent 已通过本地 tool calling 完成生成" {
		t.Fatalf("unexpected remote task summary: %s", result.GetSummary())
	}
}

func TestRunToolCallingLoopUsesToolMessages(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := &langChainAgentRuntime{
		log: log.NewHelper(logger),
	}
	model := &scriptedToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name: "read_file",
			},
		}},
		Handlers: map[string]toolcatalog.Handler{
			"read_file": func(_ context.Context, input string) (string, error) {
				return "path: " + input + "\n1: phase one goal", nil
			},
		},
	}

	result, err := runtime.runToolCallingLoop(
		context.Background(),
		model,
		toolset,
		"你是测试代理",
		"请读取 README.md 再总结",
		"tool calling loop verified",
	)
	if err != nil {
		t.Fatalf("runToolCallingLoop() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.GetSummary() != "tool calling loop verified" {
		t.Fatalf("result summary = %q, want tool calling loop verified", result.GetSummary())
	}
	if result.GetOutput() != "已读取 README.md 并完成总结" {
		t.Fatalf("result output = %q", result.GetOutput())
	}
	if model.callCount != 2 {
		t.Fatalf("GenerateContent call count = %d, want 2", model.callCount)
	}
}

func TestRunToolCallingLoopSelfCorrectsToolFailure(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := &langChainAgentRuntime{
		log: log.NewHelper(logger),
	}
	model := &selfCorrectingToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name: "read_file",
			},
		}},
		Handlers: map[string]toolcatalog.Handler{
			"read_file": func(_ context.Context, input string) (string, error) {
				if input == "configs/app.yml" {
					return "", fmt.Errorf("read file %s failed: file does not exist", input)
				}
				if input == "configs/config.yaml" {
					return "path: configs/config.yaml\n1: server:\n2: http:", nil
				}
				return "", fmt.Errorf("unexpected input: %s", input)
			},
		},
	}

	result, err := runtime.runToolCallingLoop(
		context.Background(),
		model,
		toolset,
		"你是测试代理",
		"请读取配置文件",
		"tool correction verified",
	)
	if err != nil {
		t.Fatalf("runToolCallingLoop() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil task result")
	}
	if result.GetOutput() != "已修正路径并成功读取配置文件" {
		t.Fatalf("result output = %q", result.GetOutput())
	}
	if model.callCount != 3 {
		t.Fatalf("GenerateContent call count = %d, want 3", model.callCount)
	}
}

func TestRunToolCallingLoopFailsAfterFiveCorrectionRounds(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := &langChainAgentRuntime{
		log: log.NewHelper(logger),
	}
	model := &alwaysFailingToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name: "read_file",
			},
		}},
		Handlers: map[string]toolcatalog.Handler{
			"read_file": func(_ context.Context, input string) (string, error) {
				return "", fmt.Errorf("read file %s failed: file does not exist", input)
			},
		},
	}

	result, err := runtime.runToolCallingLoop(
		context.Background(),
		model,
		toolset,
		"你是测试代理",
		"请读取缺失的文件",
		"tool correction exhausted",
	)
	if err == nil {
		t.Fatal("expected error after repeated correction failures")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
	if !strings.Contains(err.Error(), "tool self-correction exhausted after 5 rounds") {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.callCount != maxToolCorrectionRounds {
		t.Fatalf("GenerateContent call count = %d, want %d", model.callCount, maxToolCorrectionRounds)
	}
}

func TestTaskFlowWritesResultAndTrace(t *testing.T) {
	withFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	repo := NewTaskRepo(logger)
	trace := NewDelegationTraceStore()
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	dispatcher := NewTaskDispatcher(repo, runtime, trace, nil, logger)

	task := &biz.Task{
		ID:        "task-flow",
		Agent:     biz.TaskAgentDefault,
		Prompt:    "read README.md and summarize phase-one goals",
		Status:    biz.TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Save(context.Background(), task); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), task.ToCommand()); err != nil {
		t.Fatalf("dispatch task failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := repo.Get(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("get task failed: %v", err)
		}
		if stored.Status == biz.TaskStatusDone {
			if stored.Result == nil {
				t.Fatal("expected task result")
			}
			if stored.Result.GetSummary() == "" || stored.Result.GetOutput() == "" {
				t.Fatalf("unexpected task result: %+v", stored.Result)
			}
			sessions := trace.ListSessions(1)
			if len(sessions) != 1 {
				t.Fatalf("trace sessions = %d, want 1", len(sessions))
			}
			if sessions[0].Status != string(biz.TaskStatusDone) {
				t.Fatalf("trace status = %q, want done", sessions[0].Status)
			}
			if sessions[0].ResultSummary == "" || sessions[0].ResultOutput == "" {
				t.Fatalf("trace result not written back: %+v", sessions[0])
			}
			return
		}
		if stored.Status == biz.TaskStatusFailed {
			t.Fatalf("expected task done, got failed: %s", stored.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting for task flow to complete")
}

func TestDashboardStateIncludesToolTimeline(t *testing.T) {
	trace := NewDelegationTraceStore()
	trace.StartTask("session-1", biz.TaskAgentDefault, biz.TaskStatusDone)
	trace.AppendEvent(biz.DelegationEvent{
		Time:          time.Now(),
		TaskID:        "session-1",
		Agent:         biz.TaskAgentDefault.String(),
		Stage:         "tool_read_file",
		PromptPreview: "README.md",
		Summary:       "path: README.md",
	})
	trace.UpdateTask("session-1", biz.TaskStatusDone, &taskv1.TaskResult{
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
	if !strings.Contains(body, "\"result_output\":\"first phase summary\"") {
		t.Fatalf("dashboard state missing result output: %s", body)
	}
	if !strings.Contains(body, "\"stage\":\"tool_read_file\"") {
		t.Fatalf("dashboard state missing tool event: %s", body)
	}
}
