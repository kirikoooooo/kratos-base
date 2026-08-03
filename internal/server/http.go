package server

import (
	"net/http"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	dataa2a "kratos-demo/internal/data/a2a"
	"kratos-demo/internal/service"

	a2aproto "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	httptransport "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewHTTPServer)

// 普通http路由是给前端用的+业务Task，内部是用jsonrpc
func NewHTTPServer(c *conf.Server, ts *service.TaskService, ds *service.DashboardService, runtime biz.AgentRuntime, logger log.Logger) *httptransport.Server {
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
	if ts != nil {
		srv.Handle("/rpc", newTaskJSONRPCHandler(ts))
	}
	if runtime != nil {
		registerA2A(srv, c, runtime)
	}
	if ds != nil {
		ds.Register(srv)
	}
	return srv
}

func registerA2A(srv interface{ Handle(string, http.Handler) }, c *conf.Server, runtime biz.AgentRuntime) {
	handler := dataa2a.NewHandler(runtime)
	srv.Handle("/a2a", a2asrv.NewJSONRPCHandler(handler))
	card := &a2aproto.AgentCard{
		Name: "Kratos Agent Runtime", Version: "1.0.0",
		Description:       "Router, coder, reviewer, and default agents.",
		DefaultInputModes: []string{"text/plain"}, DefaultOutputModes: []string{"text/plain"},
		SupportedInterfaces: []*a2aproto.AgentInterface{a2aproto.NewAgentInterface(a2aEndpoint(c), a2aproto.TransportProtocolJSONRPC)},
	}
	srv.Handle("/.well-known/agent-card.json", a2asrv.NewStaticAgentCardHandler(card))
}

func a2aEndpoint(c *conf.Server) string {
	if c == nil || c.GetHttp() == nil || c.GetHttp().GetAddr() == "" {
		return "/a2a"
	}
	return "http://" + c.GetHttp().GetAddr() + "/a2a"
}
