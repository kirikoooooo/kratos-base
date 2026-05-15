package server

import (
	"time"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	grpctransport "github.com/go-kratos/kratos/v2/transport/grpc"
)

func NewGRPCServer(c *conf.Server, logger log.Logger) *grpctransport.Server {
	timeout := time.Duration(c.GetGrpc().GetTimeout()) * time.Second
	srv := grpctransport.NewServer(
		grpctransport.Network(c.GetGrpc().GetNetwork()),
		grpctransport.Address(c.GetGrpc().GetAddr()),
		grpctransport.Timeout(timeout),
		grpctransport.Middleware(
			logging.Server(logger),
			recovery.Recovery(),
		),
	)
	return srv
}
