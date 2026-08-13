package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"kratos-demo/app/dashboard/internal/service"
)

const defaultHTTPAddress = "127.0.0.1:8001"

// HTTPServer hosts the Dashboard control-plane facade. Task execution remains
// owned by agent-runtime and is reached through the service's gRPC client.
type HTTPServer struct{ server *http.Server }

func NewHTTPServer(tasks *service.TaskService) *HTTPServer {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if tasks != nil {
		tasks.RegisterHTTP(mux)
	}
	return &HTTPServer{server: &http.Server{Addr: httpAddress(), Handler: mux}}
}

func (s *HTTPServer) Run(ctx context.Context) error {
	if s == nil || s.server == nil {
		return errors.New("dashboard HTTP server is not available")
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func httpAddress() string {
	if address := strings.TrimSpace(os.Getenv("DASHBOARD_HTTP_ADDR")); address != "" {
		return address
	}
	return defaultHTTPAddress
}
