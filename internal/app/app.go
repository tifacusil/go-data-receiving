// Package app is the composition root of the data-receiving microservice.
//
// It is the Go equivalent of the C# Program.cs: it loads configuration, builds
// the parser, sink and dead-letter writer, and runs the TCP server (plus, in
// OutputMode=rabbitmq, the Redis buffer / RabbitMQ publisher / RetryWorker —
// and, from H4, Consul + health) as goroutines under an errgroup with a
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
	"github.com/AngelloFD/go-data-receiving/internal/discovery"
	"github.com/AngelloFD/go-data-receiving/internal/health"
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

	g, ctx := errgroup.WithContext(ctx)
	status := tcp.NewStatus()

	// In rabbitmq mode the sink is backed by Redis + RabbitMQ and the
	// RetryWorker, health endpoint and Consul registration run alongside the TCP
	// server. Other modes use a local sink only and skip those workers.
	var out sink.Sink
	if cfg.OutputMode == "rabbitmq" {
		rmq, err := newRabbitMQStack(cfg, logger)
		if err != nil {
			return err
		}
		defer rmq.close(logger)
		out = rmq.sink
		g.Go(func() error { return rmq.retry.Run(ctx) })

		// Health check verifies the TCP listener AND pings Redis + RabbitMQ.
		checker := health.NewChecker(status, rmq.redis, rmq.rmqPing)
		healthSrv := health.NewServer(cfg.Consul.HealthPort, checker, logger)
		g.Go(func() error { return healthSrv.Run(ctx) })

		reg, err := discovery.New(cfg.Consul, logger)
		if err != nil {
			return err
		}
		g.Go(func() error { return reg.Run(ctx) })
	} else {
		out, err = buildLocalSink(cfg)
		if err != nil {
			return err
		}
	}

	server := tcp.NewServer(cfg.Port, parser, out, dlx, status, logger)
	g.Go(func() error { return server.Run(ctx) })

	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return err
	}
	logger.InfoContext(context.Background(), "stopped")
	return nil
}

// buildLocalSink selects a no-dependency sink from OutputMode.
func buildLocalSink(cfg config.Config) (sink.Sink, error) {
	switch cfg.OutputMode {
	case "file":
		return sink.NewFile(cfg.OutputFilePath)
	case "null":
		return sink.NewNull(), nil
	case "stdout":
		return sink.NewStdout(), nil
	default:
		return sink.NewStdout(), nil
	}
}
