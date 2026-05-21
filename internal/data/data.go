package data

import (
	"context"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewData,
	NewTaskRepo,
	NewDelegationTraceStore,
	NewSessionChangeStore,
	NewAgentMemoryStore,
	NewAgentMemoryConfig,
	NewAgentMemoryUsecaseProvider,
	NewAgentRuntime,
	NewTaskDispatcher,
)

func NewAgentMemoryUsecaseProvider(store biz.AgentMemoryStore, cfg biz.AgentMemoryConfig) (*biz.AgentMemoryUsecase, error) {
	if err := BootstrapUserMemoryIfEmpty(context.Background(), store, cfg.UserID, ""); err != nil {
		return nil, err
	}
	return biz.NewAgentMemoryUsecase(store, cfg), nil
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
