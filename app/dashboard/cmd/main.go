package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"kratos-demo/app/dashboard/internal/data"
	"kratos-demo/app/dashboard/internal/server"
	"kratos-demo/app/dashboard/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, closeClient, err := data.NewRuntimeTaskClient(ctx, data.RuntimeGRPCAddress())
	if err != nil {
		log.Fatalf("connect agent-runtime: %v", err)
	}
	defer closeClient()
	httpServer := server.NewHTTPServer(service.NewTaskService(client))
	if err := httpServer.Run(ctx); err != nil {
		log.Fatalf("dashboard stopped: %v", err)
	}
}
