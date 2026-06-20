package messaging

import (
	"context"
	"errors"

	amqp "github.com/rabbitmq/amqp091-go"
)

// HealthPinger verifies the RabbitMQ connection is usable for the health check.
// It actively opens and closes a throwaway channel rather than just reading the
// connection's closed flag, so a half-broken broker is detected.
type HealthPinger struct {
	conn *amqp.Connection
}

// NewHealthPinger wraps an AMQP connection for health probing.
func NewHealthPinger(conn *amqp.Connection) *HealthPinger {
	return &HealthPinger{conn: conn}
}

// Ping returns nil if the broker connection can open a channel.
func (h *HealthPinger) Ping(_ context.Context) error {
	if h.conn == nil || h.conn.IsClosed() {
		return errors.New("rabbitmq connection closed")
	}
	ch, err := h.conn.Channel()
	if err != nil {
		return err
	}
	return ch.Close()
}
