package server

import (
	"os"
	"strings"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/app/agent-runtime/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	grpctransport "github.com/go-kratos/kratos/v2/transport/grpc"
)

const defaultInternalGRPCAddress = "127.0.0.1:9000"

// NewInternalGRPCServer exposes service-to-service APIs only on loopback.
// Public clients continue to use HTTP, A2A JSON-RPC, or the task JSON-RPC API.
func NewInternalGRPCServer(ts *service.TaskService, logger log.Logger) *grpctransport.Server {
	srv := grpctransport.NewServer(
		grpctransport.Network("tcp"),
		grpctransport.Address(internalGRPCAddress()),
		grpctransport.Timeout(time.Second),
		grpctransport.Middleware(logging.Server(logger), recovery.Recovery()),
	)
	if ts != nil {
		taskv1.RegisterInternalTaskServiceServer(srv.Server, ts)
	}
	return srv
}

func internalGRPCAddress() string {
	if addr := strings.TrimSpace(os.Getenv("KRATOS_INTERNAL_GRPC_ADDR")); addr != "" {
		return addr
	}
	return defaultInternalGRPCAddress
}
