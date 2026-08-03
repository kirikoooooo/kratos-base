// Package a2a adapts the local runtime to the A2A JSON-RPC protocol.
package a2a

import (
	"context"
	"iter"
	"strings"

	a2aproto "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/consts/public"
)

const AgentMetadataKey = "agent"

type executor struct {
	runtime  biz.AgentRuntime
	dispatch func(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
}

func NewHandler(runtime biz.AgentRuntime) a2asrv.RequestHandler {
	return NewHandlerWithDispatch(runtime, runtime.SendTask)
}

// NewHandlerWithDispatch lets in-process callers use ReceiveTask while the
// network endpoint continues to enqueue work through the Actor mailbox.
func NewHandlerWithDispatch(runtime biz.AgentRuntime, dispatch func(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)) a2asrv.RequestHandler {
	return a2asrv.NewHandler(&executor{runtime: runtime, dispatch: dispatch})
}

func (e *executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2aproto.Event, error] {
	return func(yield func(a2aproto.Event, error) bool) {
		prompt := messageText(execCtx.Message)
		agent := agentFromMessage(execCtx.Message)
		result, err := e.dispatch(ctx, &taskv1.TaskCommand{
			TaskID: taskIDFromMessage(execCtx.Message, string(execCtx.TaskID)),
			Agent:  string(agent),
			Prompt: prompt,
		})
		if err != nil {
			yield(a2aproto.NewStatusUpdateEvent(execCtx, a2aproto.TaskStateFailed, a2aproto.NewMessageForTask(a2aproto.MessageRoleAgent, execCtx, a2aproto.NewTextPart(err.Error()))), nil)
			return
		}
		output := ""
		if result != nil {
			output = result.Output
		}
		yield(a2aproto.NewMessageForTask(a2aproto.MessageRoleAgent, execCtx, a2aproto.NewTextPart(output)), nil)
	}
}

func (e *executor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2aproto.Event, error] {
	return func(yield func(a2aproto.Event, error) bool) {
		yield(a2aproto.NewStatusUpdateEvent(execCtx, a2aproto.TaskStateCanceled, nil), nil)
	}
}

func agentFromMessage(message *a2aproto.Message) public.AgentKind {
	if message != nil && message.Metadata != nil {
		if value, ok := message.Metadata[AgentMetadataKey].(string); ok {
			return public.AgentKind(strings.TrimSpace(value))
		}
	}
	return public.AgentKindDefault
}

func taskIDFromMessage(message *a2aproto.Message, fallback string) string {
	if message != nil && message.Metadata != nil {
		if value, ok := message.Metadata["task_id"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fallback
}

func messageText(message *a2aproto.Message) string {
	if message == nil {
		return ""
	}
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		if text := strings.TrimSpace(part.Text()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
