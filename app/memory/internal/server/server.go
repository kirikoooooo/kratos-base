package server

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	memoryv1 "kratos-demo/api/memory/v1"
	"kratos-demo/app/memory/internal/service"

	"google.golang.org/grpc"
)

const defaultGRPCAddress = "127.0.0.1:9002"

type GRPCServer struct{ server *grpc.Server }

func NewGRPCServer(memory *service.MemoryService) *GRPCServer {
	grpcServer := grpc.NewServer()
	memoryv1.RegisterMemoryServiceServer(grpcServer, memory)
	return &GRPCServer{server: grpcServer}
}

func (s *GRPCServer) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", grpcAddress())
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		s.server.GracefulStop()
	}()
	err = s.server.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

func grpcAddress() string {
	if address := strings.TrimSpace(os.Getenv("MEMORY_GRPC_ADDR")); address != "" {
		return address
	}
	return defaultGRPCAddress
}
