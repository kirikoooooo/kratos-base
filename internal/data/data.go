package data

import (
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// 如果需要切换只需要修改这里传入具体实例的构造函数
var ProviderSet = wire.NewSet(NewData, NewTaskRepo, NewAgentRuntime, NewTaskDispatcher)

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
