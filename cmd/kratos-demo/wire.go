//go:build wireinject
// +build wireinject

package main

import (
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/data"
	"kratos-demo/internal/server"
	"kratos-demo/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(serverConf *conf.Server, dataConf *conf.Data, aiConf *conf.AI, runtimeConf *conf.Runtime, logger log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newApp,
	))
}

func wireCLI(dataConf *conf.Data, aiConf *conf.AI, runtimeConf *conf.Runtime, logger log.Logger) (*service.CLIService, func(), error) {
	panic(wire.Build(
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
	))
}
