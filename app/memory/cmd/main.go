package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"kratos-demo/app/memory/internal/data"
	"kratos-demo/app/memory/internal/server"
	"kratos-demo/app/memory/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	memory, cleanup, err := data.NewPromptContextReader()
	if err != nil {
		log.Fatalf("initialize memory store: %v", err)
	}
	defer cleanup()

	if err := server.NewGRPCServer(service.NewMemoryService(memory)).Run(ctx); err != nil {
		log.Fatalf("memory stopped: %v", err)
	}
}
