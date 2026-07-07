package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	datatrace "kratos-demo/internal/data/trace"
	actorpkg "kratos-demo/third_party/actor"
	toolcatalog "kratos-demo/third_party/tools"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	grpcserver "google.golang.org/grpc"
)

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
		toolResp, ok := messages[3].Parts[0].(llms.ToolCallResponse)
		if !ok {
			return nil, fmt.Errorf("tool response part type = %T, want llms.ToolCallResponse", messages[3].Parts[0])
		}
		if !strings.Contains(toolResp.Content, "README.md") {
			return nil, fmt.Errorf("tool response content = %q, want README.md", toolResp.Content)
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
	if m.callCount == maxToolCorrectionRounds+1 {
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				Content: "## 失败分析\n\n文件 configs/missing.yaml 不存在，已重试 5 次仍无法读取。建议使用相对路径并确认文件存在。",
			}},
		}, nil
	}
	if m.callCount > maxToolCorrectionRounds+1 {
		return nil, fmt.Errorf("GenerateContent called too many times: %d", m.callCount)
	}
	if m.callCount > 1 {
		last := messages[len(messages)-1]
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

func TestRunToolCallingLoopUsesToolMessages(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	runtime := &langChainAgentRuntime{log: log.NewHelper(logger)}
	model := &scriptedToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{Type: "function", Function: &llms.FunctionDefinition{Name: "read_file"}}},
		Handlers: map[string]toolcatalog.Handler{
			"read_file": func(_ context.Context, input string) (string, error) {
				return "path: " + input + "\n1: phase one goal", nil
			},
		},
	}

	result, err := runtime.runToolCallingLoop(context.Background(), model, toolset, "你是测试代理", "请读取 README.md 再总结", "tool calling loop verified")
	if err != nil {
		t.Fatalf("runToolCallingLoop() error = %v", err)
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
	runtime := &langChainAgentRuntime{log: log.NewHelper(logger)}
	model := &selfCorrectingToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{Type: "function", Function: &llms.FunctionDefinition{Name: "read_file"}}},
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

	result, err := runtime.runToolCallingLoop(context.Background(), model, toolset, "你是测试代理", "请读取配置文件", "tool correction verified")
	if err != nil {
		t.Fatalf("runToolCallingLoop() error = %v", err)
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
	runtime := &langChainAgentRuntime{log: log.NewHelper(logger)}
	model := &alwaysFailingToolLoopLLM{}
	toolset := &managedToolset{
		Tools: []llms.Tool{{Type: "function", Function: &llms.FunctionDefinition{Name: "read_file"}}},
		Handlers: map[string]toolcatalog.Handler{
			"read_file": func(_ context.Context, input string) (string, error) {
				return "", fmt.Errorf("read file %s failed: file does not exist", input)
			},
		},
	}

	result, err := runtime.runToolCallingLoop(context.Background(), model, toolset, "你是测试代理", "请读取缺失的文件", "tool correction exhausted")
	if err != nil {
		t.Fatalf("runToolCallingLoop() error = %v", err)
	}
	if result == nil || !strings.Contains(result.GetOutput(), "失败分析") {
		t.Fatalf("expected failure analysis output, got %#v", result)
	}
	if !strings.Contains(result.GetSummary(), "任务未能完成") {
		t.Fatalf("summary = %q, want failure summary", result.GetSummary())
	}
	if model.callCount != maxToolCorrectionRounds+1 {
		t.Fatalf("GenerateContent call count = %d, want %d", model.callCount, maxToolCorrectionRounds+1)
	}
}

func TestAgentRuntimeSendTaskViaActor(t *testing.T) {
	WithFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	trace := datatrace.NewDelegationTraceStore()
	runtime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	defer actorpkg.StopActor(runtime.PID())

	if err := actorpkg.RegisterActor(runtime, 128); err != nil {
		t.Fatalf("register agent runtime failed: %v", err)
	}

	result, err := runtime.SendTask(context.Background(), &taskv1.TaskCommand{
		TaskID: "task-sync",
		Agent:  string(biz.AgentKindCoder),
		Prompt: "implement a minimal example",
	})
	if err != nil {
		t.Fatalf("expected actor send task success, got error: %v", err)
	}
	if result == nil || result.Summary == "" {
		t.Fatal("expected non-empty task result summary")
	}
}

func TestDispatchSubTaskViaRemoteGRPC(t *testing.T) {
	WithFakeRuntimeLLM(t)

	logger := log.NewStdLogger(io.Discard)
	trace := datatrace.NewDelegationTraceStore()
	remoteRuntime := NewAgentRuntime(&conf.AI{}, &conf.Runtime{}, trace, nil, nil, logger)
	remoteService := &testAgentRuntimeGRPC{runtime: remoteRuntime}

	grpcSrv := grpcserver.NewServer()
	taskv1.RegisterAgentRuntimeServiceServer(grpcSrv, remoteService)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen grpc server failed: %v", err)
	}
	defer listener.Close()

	serveDone := make(chan error, 1)
	go func() { serveDone <- grpcSrv.Serve(listener) }()
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
			Agent:   string(biz.AgentKindCoder),
			Target:  listener.Addr().String(),
			Timeout: 3,
		}},
	}, trace, nil, nil, logger)

	result, err := runtime.(*langChainAgentRuntime).dispatchSubTask(context.Background(), biz.AgentKindCoder, "remote implement a minimal endpoint")
	if err != nil {
		t.Fatalf("dispatch remote grpc sub task failed: %v", err)
	}
	if result == nil || result.GetSummary() == "" || result.GetOutput() == "" {
		t.Fatalf("unexpected remote task result: %+v", result)
	}
}

type testAgentRuntimeGRPC struct {
	taskv1.UnimplementedAgentRuntimeServiceServer
	runtime biz.AgentRuntime
}

func (s *testAgentRuntimeGRPC) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return s.runtime.ReceiveTask(ctx, cmd)
}