package server

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	grpctransport "github.com/go-kratos/kratos/v2/transport/grpc"
	httptransport "github.com/go-kratos/kratos/v2/transport/http"
)

func NewApp(logger log.Logger, gs *grpctransport.Server, hs *httptransport.Server) *kratos.App {
	return kratos.New(
		kratos.Name("kratos-demo"),
		kratos.Version("0.1.0"),
		kratos.Logger(logger),
		kratos.Server(gs, hs),
	)
}
