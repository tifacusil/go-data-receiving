// Package app is the composition root of the data-receiving microservice.
//
// It is the Go equivalent of the C# Program.cs: it loads configuration, builds
// the parser, the sink and (only in OutputMode=rabbitmq) the Redis buffer,
// RabbitMQ publisher, Consul registration and health endpoint, then runs them
// as goroutines coordinated by an errgroup and a cancellable context.
//
// The wiring is filled in incrementally across the milestones in ROADMAP.md.
// As of H0 (bootstrap) Run only validates that the binary builds and starts; the
// real composition root arrives with H2 (TCP + sinks).
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// Run is the composition root. It blocks until ctx is cancelled (SIGINT/SIGTERM)
// or a worker returns an error.
//
// TODO(H2): load config, build sink/parser, start the TCP server.
// TODO(H3): in OutputMode=rabbitmq, start the Redis buffer, RabbitMQ publisher
// and RetryWorker.
// TODO(H4): start the health endpoint and register in Consul.
func Run(ctx context.Context, version string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "data-receiving"),
		slog.String("version", version),
	)
	logger.InfoContext(ctx, "bootstrap: service starting (H0 skeleton — no workers wired yet)")

	<-ctx.Done()

	logger.InfoContext(ctx, "bootstrap: shutdown signal received, exiting")
	if err := ctx.Err(); err != nil && err != context.Canceled {
		return fmt.Errorf("app run: %w", err)
	}
	return nil
}
