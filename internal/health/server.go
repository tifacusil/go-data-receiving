package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server runs the embedded /health HTTP endpoint that Consul polls (and, when
// attached, the Prometheus /metrics endpoint). Port of the C# HealthEndpoint
// BackgroundService, using net/http instead of HttpListener.
type Server struct {
	port    int
	checker *Checker
	metrics http.Handler
	logger  *slog.Logger
}

// NewServer wires the health server.
func NewServer(port int, checker *Checker, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{port: port, checker: checker, logger: logger}
}

// WithMetrics attaches a Prometheus handler served at /metrics on the same port.
func (s *Server) WithMetrics(h http.Handler) *Server {
	s.metrics = h
	return s
}

// Run serves /health/ until ctx is cancelled. It blocks; run it in a goroutine
// from the composition root.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	// Register both /health and /health/ so Consul's trailing-slash URL and a
	// bare probe both work (the C# listener used the /health/ prefix).
	mux.HandleFunc("/health/", s.handle)
	mux.HandleFunc("/health", s.handle)
	if s.metrics != nil {
		mux.Handle("/metrics", s.metrics)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		s.logger.InfoContext(ctx, "health endpoint listening", slog.Int("port", s.port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errc:
		return fmt.Errorf("health server: %w", err)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	code, body := s.checker.Check(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
