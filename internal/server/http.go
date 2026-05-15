package server

import (
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	httptransport "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer)

func NewHTTPServer(c *conf.Server, ts *service.TaskService, ds *service.DashboardService, logger log.Logger) *httptransport.Server {
	timeout := time.Duration(c.GetHttp().GetTimeout()) * time.Second
	srv := httptransport.NewServer(
		httptransport.Network(c.GetHttp().GetNetwork()),
		httptransport.Address(c.GetHttp().GetAddr()),
		httptransport.Timeout(timeout),
		httptransport.Middleware(
			logging.Server(logger),
			recovery.Recovery(),
		),
	)
	taskv1.RegisterTaskServiceHTTPServer(srv, ts)
	if ds != nil {
		ds.Register(srv)
	}
	return srv
}
