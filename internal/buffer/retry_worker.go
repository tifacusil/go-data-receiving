package buffer

import (
	"context"
	"log/slog"
	"time"
)

// Republisher publishes a buffered envelope back to the broker. Declared here
// (consumer side) and satisfied by messaging.RabbitMQ.
type Republisher interface {
	PublishJSON(ctx context.Context, routingKey, envelopeJSON, messageID string) error
}

// RetryWorker periodically rescans the Redis buffer and republishes entries that
// have been stuck longer than minAge (e.g. because RabbitMQ was down). Port of
// the C# RetryWorker BackgroundService.
//
// TODO(SE-practice): the baseline retries at a fixed interval with no exponential
// backoff or attempt cap (CLAUDE.md §4 asks for 1s/5s/30s, max 3). Kept 1:1 with
// C# so the Go/C# performance comparison stays fair; revisit if the experiment
// allows behavioral divergence.
type RetryWorker struct {
	buffer   Buffer
	pub      Republisher
	interval time.Duration
	minAge   time.Duration
	logger   *slog.Logger
}

// NewRetryWorker wires a RetryWorker.
func NewRetryWorker(b Buffer, pub Republisher, interval, minAge time.Duration, logger *slog.Logger) *RetryWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetryWorker{buffer: b, pub: pub, interval: interval, minAge: minAge, logger: logger}
}

// Run loops until ctx is cancelled, executing a retry pass every interval.
func (w *RetryWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.runPass(ctx)
		}
	}
}

// runPass scans the buffer once and republishes everything older than minAge.
func (w *RetryWorker) runPass(ctx context.Context) {
	if count, err := w.buffer.Count(ctx); err == nil && count > 0 {
		w.logger.InfoContext(ctx, "retry pass", slog.Int64("buffered", count))
	}

	msgs, err := w.buffer.ScanOld(ctx, w.minAge)
	if err != nil {
		w.logger.ErrorContext(ctx, "retry scan failed", slog.Any("error", err))
		return
	}

	var retried, succeeded int
	for _, m := range msgs {
		retried++
		if err := w.pub.PublishJSON(ctx, m.RoutingKey, m.EnvelopeJSON, m.MessageID); err != nil {
			w.logger.WarnContext(ctx, "retry publish failed",
				slog.String("message_id", m.MessageID), slog.Any("error", err))
			continue
		}
		if err := w.buffer.Delete(ctx, m.MessageID); err != nil {
			w.logger.WarnContext(ctx, "retry delete failed",
				slog.String("message_id", m.MessageID), slog.Any("error", err))
			continue
		}
		succeeded++
		w.logger.InfoContext(ctx, "retry succeeded", slog.String("message_id", m.MessageID))
	}
	if retried > 0 {
		w.logger.InfoContext(ctx, "retry pass complete",
			slog.Int("retried", retried), slog.Int("published", succeeded))
	}
}
