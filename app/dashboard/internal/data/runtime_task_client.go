package data

import (
	"context"
	"os"
	"strings"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/app/dashboard/internal/biz"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultRuntimeGRPCAddress = "127.0.0.1:9000"

// RuntimeGRPCAddress selects the agent-runtime internal endpoint. Production
// deployments should use service discovery plus TLS instead of this default.
func RuntimeGRPCAddress() string {
	if address := strings.TrimSpace(os.Getenv("AGENT_RUNTIME_GRPC_ADDR")); address != "" {
		return address
	}
	return defaultRuntimeGRPCAddress
}

type runtimeTaskClient struct {
	client taskv1.InternalTaskServiceClient
}

func NewRuntimeTaskClient(_ context.Context, address string) (biz.TaskClient, func() error, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return &runtimeTaskClient{client: taskv1.NewInternalTaskServiceClient(conn)}, conn.Close, nil
}

func (c *runtimeTaskClient) CreateTask(ctx context.Context, request *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error) {
	return c.client.CreateTask(ctx, request)
}

func (c *runtimeTaskClient) GetTask(ctx context.Context, request *taskv1.GetTaskRequest) (*taskv1.TaskReply, error) {
	return c.client.GetTask(ctx, request)
}
