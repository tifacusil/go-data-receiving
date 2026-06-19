// Package buffer is the Redis durability buffer in front of RabbitMQ: every
// envelope is written here before publishing and deleted only after the broker
// confirms, so nothing is lost if RabbitMQ is down. Port of the C# RedisMessageBuffer.
package buffer

import (
	"context"
	"time"
)

// BufferedMessage is a buffered envelope awaiting (re)publication.
type BufferedMessage struct {
	MessageID    string
	RoutingKey   string
	EnvelopeJSON string
	StoredAt     time.Time
}

// Buffer stores envelopes pending publish confirmation. Port of the C# IMessageBuffer.
type Buffer interface {
	// Set writes the envelope under sensor_buffer:{messageID} with the TTL.
	Set(ctx context.Context, messageID, routingKey, envelopeJSON string) error
	// Delete removes a buffered entry after the broker confirmed it.
	Delete(ctx context.Context, messageID string) error
	// ScanOld yields buffered messages older than minAge.
	ScanOld(ctx context.Context, minAge time.Duration) ([]BufferedMessage, error)
	// Count returns the number of buffered entries.
	Count(ctx context.Context) (int64, error)
}
