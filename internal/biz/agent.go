package biz

import (
	"context"
	"errors"
	"fmt"

	taskv1 "kratos-demo/api/task/v1"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewAgentRuntimeUsecase, NewTaskUsecase)

type AgentRuntime interface {
	// 外部模块调用接口
	actorpkg.Actor
	Name() string
	Supports(TaskAgent) bool
	Execute(context.Context, TaskAgent, string) (*taskv1.TaskResult, error)
	// 内部使用，不提供外部模块调用，比如ReceiveTask实际上是Process调用，但是必须实现
	ReceiveTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
	SendTask(context.Context, *taskv1.TaskCommand) (*taskv1.TaskResult, error)
}

type DelegationVerifier interface {
	VerifyDelegation(context.Context, string, TaskAgent, string) (*taskv1.TaskResult, error)
}

type AgentRuntimeUsecase struct {
	ar AgentRuntime
}

func NewAgentRuntimeUsecase(ar AgentRuntime) *AgentRuntimeUsecase {
	return &AgentRuntimeUsecase{ar: ar}
}

// UseCase Actor 接口方法调用（注意不是实现）
func (aru *AgentRuntimeUsecase) GetPID() (actorpkg.PID, error) {
	logMsg := fmt.Sprintf("getting PID for agent runtime: %s", aru.ar.Name())
	log.Debug(logMsg)
	pid := aru.ar.PID()
	log.Debug(logMsg)
	return pid, nil
}

func (aru *AgentRuntimeUsecase) Process(msg *actorpkg.Message) error {
	logMsg := fmt.Sprintf("processing message for agent runtime: %s", aru.ar.Name())
	log.Debug(logMsg)
	aru.ar.Process(msg) // process -> receivetask
	log.Debug(logMsg)
	return nil
}

func (aru *AgentRuntimeUsecase) OnStop() error {
	logMsg := fmt.Sprintf("stopping agent runtime: %s", aru.ar.Name())
	log.Debug(logMsg)
	aru.ar.OnStop()

	log.Debug(logMsg)
	return nil
}

// 非Actor里面的方法
func (aru *AgentRuntimeUsecase) Name() string {
	if aru == nil || aru.ar == nil {
		return ""
	}
	return aru.ar.Name()
}

func (aru *AgentRuntimeUsecase) Supports(agent TaskAgent) bool {
	if aru == nil || aru.ar == nil {
		return false
	}
	return aru.ar.Supports(agent)
}

func (aru *AgentRuntimeUsecase) Execute(ctx context.Context, agent TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	if aru == nil || aru.ar == nil {
		return nil, errors.New("agent runtime is not available")
	}
	if !aru.ar.Supports(agent) {
		return nil, fmt.Errorf("agent %s is not supported by runtime %s", agent, aru.ar.Name())
	}

	return aru.ar.Execute(ctx, agent, prompt)
}

// // 实际实现里应该被Process解析成Task 调用
func (aru *AgentRuntimeUsecase) ReceiveTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if aru == nil || aru.ar == nil {
		return nil, errors.New("agent runtime is not available")
	}
	return aru.ar.ReceiveTask(ctx, cmd)
}

func (aru *AgentRuntimeUsecase) SendTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if aru == nil || aru.ar == nil {
		return nil, errors.New("agent runtime is not available")
	}
	return aru.ar.SendTask(ctx, cmd)
}
