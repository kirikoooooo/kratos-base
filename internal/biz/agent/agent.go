package agent

import (
	"context"
	"errors"

	taskv1 "kratos-demo/api/task/v1"
	biztool "kratos-demo/internal/biz/tool"
)

// Agent 是领域层的核心聚合根，作为系统总入口管理。
// 它注入各限界上下文的接口对象，协调运行时、工具之间的交互。
//
// Memory 通过 Runtime 间接访问——langChainAgentRuntime 内部已持有 AgentMemory，
// 无需在 Agent 层重复注入。需要直接访问记忆的场景通过 Getter/Setter 模式扩展。
type Agent struct {
	// Runtime Agent 运行时实现（端口），由 data 层提供适配器。
	Runtime AgentRuntime

	// Tools 工具仓库接口（端口），提供已注册的工具绑定列表。
	Tools biztool.ToolRepository
}

// NewAgent 创建一个聚合了各限界上下文接口的 Agent 实例。
func NewAgent(runtime AgentRuntime, tools biztool.ToolRepository) *Agent {
	return &Agent{
		Runtime: runtime,
		Tools:   tools,
	}
}

// ExecuteTask handles a TaskCommand by delegating to the agent runtime.
func (a *Agent) ExecuteTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if a == nil || a.Runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return a.Runtime.ReceiveTask(ctx, cmd)
}

// Execute runs a prompt with the specified agent role.
func (a *Agent) Execute(ctx context.Context, kind AgentKind, prompt string) (*taskv1.TaskResult, error) {
	if a == nil || a.Runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	if !a.Runtime.Supports(kind) {
		return nil, ErrAgentNotSupported
	}
	return a.Runtime.Execute(ctx, kind, prompt)
}

// VerifyDelegation verifies a delegated task result.
func (a *Agent) VerifyDelegation(ctx context.Context, taskID string, kind AgentKind, prompt string) (*taskv1.TaskResult, error) {
	if a == nil || a.Runtime == nil {
		return nil, ErrAgentRuntimeUnavailable
	}
	verifier, ok := a.Runtime.(DelegationVerifier)
	if !ok {
		return nil, ErrDelegationNotSupported
	}
	return verifier.VerifyDelegation(ctx, taskID, kind, prompt)
}
