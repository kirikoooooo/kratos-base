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

func WithAgent(ctx context.Context, agent biz.Agent) context.Context {
	return context.WithValue(ctx, agentContextKey{}, agent)
}

func Agent(ctx context.Context) biz.Agent {
	if ctx == nil {
		return biz.AgentDefault
	}
	agent, _ := ctx.Value(agentContextKey{}).(biz.Agent)
	if strings.TrimSpace(string(agent)) == "" {
		return biz.AgentDefault
	}
	return agent
}
