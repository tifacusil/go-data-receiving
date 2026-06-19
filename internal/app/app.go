// Package app is the composition root of the data-receiving microservice.
//
// It is the Go equivalent of the C# Program.cs: it loads configuration, builds
// the parser, sink and dead-letter writer, and runs the TCP server (plus, in
// OutputMode=rabbitmq, the Redis buffer / RabbitMQ publisher / Consul / health
// endpoint — added in later milestones) as goroutines under an errgroup with a
// cancellable context.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/AngelloFD/go-data-receiving/internal/config"
	"github.com/AngelloFD/go-data-receiving/internal/deadletter"
	"github.com/AngelloFD/go-data-receiving/internal/parsing"
	"github.com/AngelloFD/go-data-receiving/internal/schema"
	"github.com/AngelloFD/go-data-receiving/internal/sink"
	"github.com/AngelloFD/go-data-receiving/internal/tcp"
)

// Run is the composition root. It blocks until ctx is cancelled (SIGINT/SIGTERM)
// or a worker returns an error.
func Run(ctx context.Context, version string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "data-receiving"),
		slog.String("version", version),
		slog.String("instance_id", cfg.Consul.InstanceID),
	)
	logger.InfoContext(ctx, "starting",
		slog.String("output_mode", cfg.OutputMode), slog.Int("port", cfg.Port))

	parser := parsing.NewParser(schema.New())

	dlx, err := deadletter.NewWriter(cfg.DlxPath)
	if err != nil {
		return err
	}

	out, err := buildSink(cfg, logger)
	if err != nil {
		return err
	}

	status := tcp.NewStatus()
	server := tcp.NewServer(cfg.Port, parser, out, dlx, status, logger)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return server.Run(ctx) })

	// TODO(H3): OutputMode=rabbitmq → start Redis buffer, RabbitMQ publisher, RetryWorker.
	// TODO(H4): start the health endpoint and register in Consul.

	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	logger.InfoContext(context.Background(), "stopped")
	return nil
}

// buildSink selects the output sink from OutputMode, matching the C# switch.
// rabbitmq is not wired until H3; for now it is rejected explicitly rather than
// silently degrading.
func buildSink(cfg config.Config, _ *slog.Logger) (sink.Sink, error) {
	switch cfg.OutputMode {
	case "file":
		return sink.NewFile(cfg.OutputFilePath)
	case "null":
		return sink.NewNull(), nil
	case "stdout":
		return sink.NewStdout(), nil
	case "rabbitmq":
		// TODO(H3): construct the Redis-buffered RabbitMQ sink.
		return nil, fmt.Errorf("OutputMode=rabbitmq not implemented until H3")
	default:
		return sink.NewStdout(), nil
	}
}
