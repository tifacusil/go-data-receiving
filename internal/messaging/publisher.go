// Package messaging publishes envelopes to RabbitMQ with publisher confirms.
// Port of the C# RabbitMqPublisher. The service only publishes to the
// pre-declared sensors.raw exchange; it never declares topology (the
// data_receiving broker user has no configure permission).
package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends envelopes to the broker. Port of the C# IMessagePublisher.
type Publisher interface {
	// PublishJSON publishes a serialized envelope under routingKey and blocks
	// until the broker confirms or the confirm timeout elapses.
	PublishJSON(ctx context.Context, routingKey, envelopeJSON, messageID string) error
}

// RabbitMQ is a confirm-mode Publisher over a single AMQP channel. The channel
// is not concurrency-safe, so publishes are serialized with a mutex (matching
// the C# lock).
type RabbitMQ struct {
	channel        *amqp.Channel
	exchange       string
	confirmTimeout time.Duration
	mu             sync.Mutex
}

// NewRabbitMQ puts the channel into confirm mode and returns a Publisher.
func NewRabbitMQ(channel *amqp.Channel, exchange string, confirmTimeout time.Duration) (*RabbitMQ, error) {
	if err := channel.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return &RabbitMQ{channel: channel, exchange: exchange, confirmTimeout: confirmTimeout}, nil
}

// PublishJSON publishes and waits for the broker ack.
func (r *RabbitMQ) PublishJSON(ctx context.Context, routingKey, envelopeJSON, messageID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conf, err := r.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		r.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // delivery_mode 2
			ContentType:  "application/json",
			MessageId:    messageID,
			Body:         []byte(envelopeJSON),
		},
	)
	if err != nil {
		return fmt.Errorf("publish %s: %w", routingKey, err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.confirmTimeout)
	defer cancel()
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("await confirm for %s: %w", messageID, err)
	}
	if !acked {
		return fmt.Errorf("broker nacked message %s", messageID)
	}
	return nil
}
