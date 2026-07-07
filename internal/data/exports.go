package data

import (
	"context"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	dataagent "kratos-demo/internal/data/agent_runtime"
	datasession "kratos-demo/internal/data/session"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
)

func NewTaskRepo(logger log.Logger) datatasking.TaskRepo {
	return datatasking.NewTaskRepo(logger)
}

func NewDelegationTraceStore() datatrace.DelegationTraceStore {
	return datatrace.NewDelegationTraceStore()
}

func NewSessionStore(dataConf *conf.Data) datasession.SessionStore {
	return datasession.NewSessionStore(dataConf)
}

func NewAgentMemoryStore(dataConf *conf.Data, logger log.Logger) (dataagent.AgentMemoryStore, error) {
	return dataagent.NewAgentMemoryStore(dataConf, logger)
}

func NewAgentMemoryConfig(dataConf *conf.Data) dataagent.AgentMemoryConfig {
	return dataagent.NewAgentMemoryConfig(dataConf)
}

func NewAgentRuntime(config *conf.AI, runtimeConfig *conf.Runtime, trace datatrace.DelegationTraceStore, sessions datasession.SessionStore, memory dataagent.AgentMemory, logger log.Logger) biz.AgentRuntime {
	return dataagent.NewAgentRuntime(config, runtimeConfig, trace, sessions, memory, logger)
}

func NewTaskDispatcher(repo datatasking.TaskRepo, runtime biz.AgentRuntime, trace datatrace.DelegationTraceStore, memory dataagent.AgentMemory, logger log.Logger) datatasking.TaskDispatcher {
	return datatasking.NewTaskDispatcher(repo, runtime, trace, memory, logger)
}

func NewAgentMemoryUsecase(store dataagent.AgentMemoryStore, config dataagent.AgentMemoryConfig) dataagent.AgentMemory {
	return dataagent.NewAgentMemoryUsecase(store, config)
}

func BootstrapUserMemoryIfEmpty(ctx context.Context, store dataagent.AgentMemoryStore, userID, workspace string) error {
	return dataagent.BootstrapUserMemoryIfEmpty(ctx, store, userID, workspace)
}
