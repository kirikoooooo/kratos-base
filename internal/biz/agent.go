package biz

import (
	"context"

	actorpkg "kratos-demo/third_party/actor"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewTaskUsecase)

type AgentRuntime interface {
	actorpkg.Actor

	Name() string
	Supports(TaskAgent) bool
	Execute(context.Context, TaskAgent, string) (*TaskResult, error)
}
