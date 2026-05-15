package data

import (
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewData, NewTaskRepo, NewDelegationTraceStore, NewAgentRuntime, NewTaskDispatcher)

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
