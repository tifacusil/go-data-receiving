package sink

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// bufferStore and publisher are the minimal dependencies of the RabbitMQ sink,
// declared here (consumer side) and satisfied by internal/buffer and
// internal/messaging.
type bufferStore interface {
	Set(ctx context.Context, messageID, routingKey, envelopeJSON string) error
	Delete(ctx context.Context, messageID string) error
}

type publisher interface {
	PublishJSON(ctx context.Context, routingKey, envelopeJSON, messageID string) error
}

// RabbitMQ is the durable sink: it buffers every envelope in Redis, publishes to
// RabbitMQ, then deletes the buffer entry only after the broker confirms. If the
// publish fails the entry stays in Redis for the RetryWorker — and Write still
// returns nil so the DTU receives a success ACK (1:1 with the C# RabbitMqSink,
// which swallows the publish error).
type RabbitMQ struct {
	buffer bufferStore
	pub    publisher
	logger *slog.Logger
}

// NewRabbitMQ wires the durable sink.
func NewRabbitMQ(b bufferStore, pub publisher, logger *slog.Logger) *RabbitMQ {
	if logger == nil {
		logger = slog.Default()
	}
	return &RabbitMQ{buffer: b, pub: pub, logger: logger}
}

// Write buffers, publishes, and deletes on confirm.
func (s *RabbitMQ) Write(ctx context.Context, env *parsing.Envelope) error {
	routingKey := fmt.Sprintf("sensors.%s.%s", env.ProjectID, env.SensorType)
	json := parsing.Serialize(env)

	if err := s.buffer.Set(ctx, env.MessageID, routingKey, json); err != nil {
		// Buffering is the durability guarantee; if it fails we cannot proceed
		// safely, so surface the error (server will skip the success ACK).
		return fmt.Errorf("buffer envelope %s: %w", env.MessageID, err)
	}

	if err := s.pub.PublishJSON(ctx, routingKey, json, env.MessageID); err != nil {
		s.logger.WarnContext(ctx, "rabbitmq publish failed; buffered in redis for retry",
			slog.String("message_id", env.MessageID), slog.Any("error", err))
		return nil // buffered; RetryWorker will republish
	}

	if err := s.buffer.Delete(ctx, env.MessageID); err != nil {
		s.logger.WarnContext(ctx, "buffer delete after publish failed; RetryWorker may republish",
			slog.String("message_id", env.MessageID), slog.Any("error", err))
		return nil
	}

	s.logger.DebugContext(ctx, "published",
		slog.String("sensor_type", env.SensorType), slog.String("routing_key", routingKey),
		slog.String("message_id", env.MessageID))
	return nil
}
