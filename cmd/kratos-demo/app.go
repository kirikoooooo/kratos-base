package main

import (
	"kratos-demo/internal/biz"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	grpctransport "github.com/go-kratos/kratos/v2/transport/grpc"
	httptransport "github.com/go-kratos/kratos/v2/transport/http"
)

func newApp(
	logger log.Logger,
	gs *grpctransport.Server,
	hs *httptransport.Server,
	_ *biz.OrderUsecase,
	_ *biz.PricingUsecase,
) *kratos.App {
	return kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Logger(logger),
		kratos.Server(gs, hs),
	)
}
