package ctx

import (
	"context"
	"strings"

	"kratos-demo/internal/biz"
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

func WithAgent(ctx context.Context, agent biz.AgentKind) context.Context {
	return context.WithValue(ctx, agentContextKey{}, agent)
}

func Agent(ctx context.Context) biz.AgentKind {
	if ctx == nil {
		return biz.AgentKindDefault
	}
	agent, _ := ctx.Value(agentContextKey{}).(biz.AgentKind)
	if strings.TrimSpace(string(agent)) == "" {
		return biz.AgentKindDefault
	}
	return agent
}
