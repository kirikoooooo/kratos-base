package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// StartCLISSEServer exposes a CLI service's output stream on a loopback HTTP
// listener. The caller owns the returned shutdown function.
func StartCLISSEServer(ctx context.Context, addr string, stream *CLIOutputStream) (func(context.Context) error, error) {
	if stream == nil {
		return nil, errors.New("CLI output stream is nil")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen CLI SSE server: %w", err)
	}
	mux := http.NewServeMux()
	RegisterCLIOutputStream(mux, stream)
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return server.Shutdown, nil
}
