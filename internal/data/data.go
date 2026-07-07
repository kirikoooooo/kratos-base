package data

import (
	"context"

	"kratos-demo/internal/conf"
	dataagent "kratos-demo/internal/data/agent_runtime"
	datasession "kratos-demo/internal/data/session"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewData,
	datatasking.NewTaskRepo,
	datatasking.NewTaskUsecase,
	datatrace.NewDelegationTraceStore,
	datasession.NewSessionStore,
	dataagent.NewAgentMemoryStore,
	dataagent.NewAgentMemoryConfig,
	NewAgentMemoryUsecaseProvider,
	dataagent.NewAgentRuntime,
	datatasking.NewTaskDispatcher,
)

func NewAgentMemoryUsecaseProvider(store dataagent.AgentMemoryStore, cfg dataagent.AgentMemoryConfig) (dataagent.AgentMemory, error) {
	if err := dataagent.BootstrapUserMemoryIfEmpty(context.Background(), store, cfg.UserID, ""); err != nil {
		return nil, err
	}
	return dataagent.NewAgentMemoryUsecase(store, cfg), nil
}

type Data struct {
	db  *conf.Data_Database
	log *log.Helper
}

func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	helper := log.NewHelper(logger)
	d := &Data{
		db:  c.GetDatabase(),
		log: helper,
	}
	cleanup := func() {
		helper.Info("closing data resources")
	}
	return d, cleanup, nil
}
