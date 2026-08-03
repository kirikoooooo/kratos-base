package a2a

import (
	"context"
	"testing"

	a2aproto "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/consts/public"
	actorpkg "kratos-demo/third_party/actor"
)

type runtimeStub struct{}

func (runtimeStub) PID() actorpkg.PID              { return actorpkg.NewPID(1, "runtime-stub") }
func (runtimeStub) Process(*actorpkg.Message)      {}
func (runtimeStub) OnStop()                        {}
func (runtimeStub) Name() string                   { return "stub" }
func (runtimeStub) Supports(public.AgentKind) bool { return true }
func (runtimeStub) Execute(context.Context, public.AgentKind, string) (*taskv1.TaskResult, error) {
	return nil, nil
}
func (runtimeStub) ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return nil, nil
}
func (runtimeStub) SendTask(_ context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	return &taskv1.TaskResult{Output: cmd.Prompt}, nil
}

func TestHandlerExecutesA2AMessageThroughRuntime(t *testing.T) {
	h := NewHandler(runtimeStub{})
	message := a2aproto.NewMessage(a2aproto.MessageRoleUser, a2aproto.NewTextPart("implement endpoint"))
	message.Metadata = map[string]any{"agent": "coder"}
	result, err := h.SendMessage(t.Context(), &a2aproto.SendMessageRequest{Message: message})
	if err != nil {
		t.Fatal(err)
	}
	message, ok := result.(*a2aproto.Message)
	if !ok || message.Parts[0].Text() != "implement endpoint" {
		t.Fatalf("result = %#v", result)
	}
}

var _ biz.AgentRuntime = runtimeStub{}
var _ a2asrv.AgentExecutor = (*executor)(nil)
