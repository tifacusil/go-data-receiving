package app

import (
	"context"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/AngelloFD/go-data-receiving/internal/buffer"
	"github.com/AngelloFD/go-data-receiving/internal/config"
	"github.com/AngelloFD/go-data-receiving/internal/messaging"
	"github.com/AngelloFD/go-data-receiving/internal/sink"
)

// rabbitmqStack holds the rabbitmq-mode dependencies and their teardown.
type rabbitmqStack struct {
	redis   *buffer.Redis
	conn    *amqp.Connection
	channel *amqp.Channel
	sink    sink.Sink
	retry   *buffer.RetryWorker
	rmqPing *messaging.HealthPinger
}

// newRabbitMQStack builds the Redis buffer, the AMQP connection/channel, the
// confirm-mode publisher, the durable sink and the RetryWorker. Mirrors the
// rabbitmq branch of the C# Program.cs DI setup.
func newRabbitMQStack(cfg config.Config, logger *slog.Logger) (*rabbitmqStack, error) {
	redisBuf, err := buffer.NewRedis(cfg.Redis.ConnectionString, cfg.Redis.BufferTTL())
	if err != nil {
		return nil, fmt.Errorf("init redis buffer: %w", err)
	}
	if err := redisBuf.Ping(context.Background()); err != nil {
		_ = redisBuf.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	conn, err := amqp.DialConfig(amqpURL(cfg.RabbitMQ), amqp.Config{
		Properties: amqp.Table{"connection_name": "data-receiving"},
	})
	if err != nil {
		_ = redisBuf.Close()
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		_ = redisBuf.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}

	pub, err := messaging.NewRabbitMQ(channel, cfg.RabbitMQ.Exchange, cfg.RabbitMQ.ConfirmTimeout())
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		_ = redisBuf.Close()
		return nil, err
	}

	out := sink.NewRabbitMQ(redisBuf, pub, logger)
	retry := buffer.NewRetryWorker(redisBuf, pub, cfg.Redis.RetryInterval(), cfg.Redis.RetryMinAge(), logger)

	return &rabbitmqStack{
		redis:   redisBuf,
		conn:    conn,
		channel: channel,
		sink:    out,
		retry:   retry,
		rmqPing: messaging.NewHealthPinger(conn),
	}, nil
}

// close tears down the stack in reverse order.
func (s *rabbitmqStack) close(logger *slog.Logger) {
	if err := s.channel.Close(); err != nil {
		logger.Warn("closing amqp channel", slog.Any("error", err))
	}
	if err := s.conn.Close(); err != nil {
		logger.Warn("closing amqp connection", slog.Any("error", err))
	}
	if err := s.redis.Close(); err != nil {
		logger.Warn("closing redis", slog.Any("error", err))
	}
}

func amqpURL(c config.RabbitMQConfig) string {
	return fmt.Sprintf("amqp://%s:%s@%s:%d%s",
		c.Username, c.Password, c.Host, c.Port, normalizeVHost(c.VirtualHost))
}

// normalizeVHost renders the vhost for the AMQP URL. "/" (the default vhost)
// becomes "/%2F" so it is not parsed as an empty path.
func normalizeVHost(vhost string) string {
	if vhost == "/" {
		return "/%2F"
	}
	return "/" + vhost
}
