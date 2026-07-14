package ctx

import (
	"context"
	"strings"

	"kratos-demo/internal/consts/public"
)

type taskIDContextKey struct{}
type agentContextKey struct{}

func WithTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskIDContextKey{}, taskID)
}

func TaskID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	taskID, _ := ctx.Value(taskIDContextKey{}).(string)
	return taskID
}

func WithAgent(ctx context.Context, agent public.AgentKind) context.Context {
	return context.WithValue(ctx, agentContextKey{}, agent)
}

func Agent(ctx context.Context) public.AgentKind {
	if ctx == nil {
		return public.AgentKindDefault
	}
	agent, _ := ctx.Value(agentContextKey{}).(public.AgentKind)
	if strings.TrimSpace(string(agent)) == "" {
		return public.AgentKindDefault
	}
	return agent
}
