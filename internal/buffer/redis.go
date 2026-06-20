package buffer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "sensor_buffer:"

// wrapper is the JSON stored per buffered message. snake_case field names match
// the C# BufferWrapper so the on-disk Redis format is identical. StoredAt is
// written and read only by this Go service, so RFC3339Nano round-trips cleanly.
type wrapper struct {
	StoredAt     time.Time `json:"stored_at"`
	RoutingKey   string    `json:"routing_key"`
	MessageID    string    `json:"message_id"`
	EnvelopeJSON string    `json:"envelope_json"`
}

// Redis is a Buffer backed by a single Redis instance.
type Redis struct {
	client *redis.Client
	ttl    time.Duration
	now    func() time.Time
}

// NewRedis builds a Buffer from a connection string of the form
// "host:6379,password=secret" (the C#/StackExchange.Redis format) or a standard
// redis URL. ttl is the per-entry expiry.
func NewRedis(connectionString string, ttl time.Duration) (*Redis, error) {
	opt, err := parseConnString(connectionString)
	if err != nil {
		return nil, err
	}
	return &Redis{client: redis.NewClient(opt), ttl: ttl, now: time.Now}, nil
}

// Ping verifies connectivity (used by the health check in H4).
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close releases the connection pool.
func (r *Redis) Close() error { return r.client.Close() }

// Set stores the envelope under sensor_buffer:{messageID} with the TTL.
func (r *Redis) Set(ctx context.Context, messageID, routingKey, envelopeJSON string) error {
	w := wrapper{
		StoredAt:     r.now().UTC(),
		RoutingKey:   routingKey,
		MessageID:    messageID,
		EnvelopeJSON: envelopeJSON,
	}
	value, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal buffer wrapper: %w", err)
	}
	return r.client.Set(ctx, keyPrefix+messageID, value, r.ttl).Err()
}

// Delete removes a buffered entry.
func (r *Redis) Delete(ctx context.Context, messageID string) error {
	return r.client.Del(ctx, keyPrefix+messageID).Err()
}

// ScanOld returns buffered messages older than minAge. It SCANs the keyspace
// (non-blocking, paged) like the C# IServer.Keys iteration.
func (r *Redis) ScanOld(ctx context.Context, minAge time.Duration) ([]BufferedMessage, error) {
	cutoff := r.now().UTC().Add(-minAge)
	var out []BufferedMessage
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, keyPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan buffer: %w", err)
		}
		for _, key := range keys {
			val, err := r.client.Get(ctx, key).Result()
			if errors.Is(err, redis.Nil) {
				continue // expired between SCAN and GET
			}
			if err != nil {
				return nil, fmt.Errorf("get %s: %w", key, err)
			}
			var w wrapper
			if json.Unmarshal([]byte(val), &w) != nil {
				continue // corrupt entry, skip (matches C# try/catch)
			}
			if w.StoredAt.After(cutoff) {
				continue // too young
			}
			out = append(out, BufferedMessage{
				MessageID:    w.MessageID,
				RoutingKey:   w.RoutingKey,
				EnvelopeJSON: w.EnvelopeJSON,
				StoredAt:     w.StoredAt,
			})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return out, nil
}

// Count returns the number of buffered entries by SCANning the keyspace.
func (r *Redis) Count(ctx context.Context) (int64, error) {
	var count int64
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, keyPrefix+"*", 100).Result()
		if err != nil {
			return 0, fmt.Errorf("scan buffer: %w", err)
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count, nil
}

// parseConnString accepts the StackExchange.Redis style "host:port,password=...,key=val"
// used by the C# config and the Wave 1 secrets, and maps it onto redis.Options.
func parseConnString(s string) (*redis.Options, error) {
	opt := &redis.Options{}
	first := true
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			switch k {
			case "password":
				opt.Password = v
			case "user", "username":
				opt.Username = v
			default:
				// ignore unsupported StackExchange options (ssl, abortConnect, ...)
			}
			continue
		}
		if first {
			opt.Addr = part
			first = false
		}
	}
	if opt.Addr == "" {
		return nil, fmt.Errorf("redis connection string %q has no host:port", s)
	}
	return opt, nil
}
