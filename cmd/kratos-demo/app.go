package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	httptransport "github.com/go-kratos/kratos/v2/transport/http"
)

func newApp(
	logger log.Logger,
	hs *httptransport.Server,
) *kratos.App {
	return kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Logger(logger),
		kratos.Server(hs),
	)
}
