//go:build integration

// Package integration exercises the Redis buffer + RabbitMQ publisher against
// real services. Run with: go test -tags=integration ./test/integration/...
//
// Connection defaults target the local landslide-infra stack; override via env:
//
//	RECEIVER__REDIS__CONNECTIONSTRING   (default localhost:6379,password=changeme_redis)
//	IT_RABBITMQ_URL                     (default amqp://data_receiving:changeme_data_receiving@localhost:5672/)
//	IT_RABBITMQ_ADMIN_URL               (default amqp://admin:XLrCTo51s4n50qy8GQ@localhost:5672/)
package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/AngelloFD/go-data-receiving/internal/buffer"
	"github.com/AngelloFD/go-data-receiving/internal/messaging"
	"github.com/AngelloFD/go-data-receiving/internal/parsing"
	"github.com/AngelloFD/go-data-receiving/internal/sink"
)

const (
	persistQueue = "data_management.persist" // bound to sensors.raw with sensors.#
	exchange     = "sensors.raw"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func redisConn() string {
	return envOr("RECEIVER__REDIS__CONNECTIONSTRING", "localhost:6379,password=changeme_redis")
}
func rabbitURL() string {
	return envOr("IT_RABBITMQ_URL", "amqp://data_receiving:changeme_data_receiving@localhost:5672/")
}
func adminURL() string {
	return envOr("IT_RABBITMQ_ADMIN_URL", "amqp://admin:XLrCTo51s4n50qy8GQ@localhost:5672/")
}

func sampleEnvelope(msgID string) *parsing.Envelope {
	return &parsing.Envelope{
		SchemaVersion: "1.0", MessageID: msgID, ProjectID: "shangtan", SensorID: "GXZM-DB03",
		SensorType: "gnss", Timestamp: "2026-05-02T14:30:00Z", ReceivedAt: "2026-05-02T14:30:12Z",
		SamplingIntervalSeconds: 3600,
		Payload: parsing.GnssPayload{
			DisplacementHorizontalMm: 13.57, CumulativeDisplacementMm: 1247.5, FixQuality: "rtk_fixed",
		},
	}
}

// purgeQueue drains leftover messages so each test starts clean.
func purgeQueue(t *testing.T, ch *amqp.Channel) {
	t.Helper()
	if _, err := ch.QueuePurge(persistQueue, false); err != nil {
		t.Logf("purge %s: %v", persistQueue, err)
	}
}

func newBuffer(t *testing.T) *buffer.Redis {
	t.Helper()
	b, err := buffer.NewRedis(redisConn(), 24*time.Hour)
	if err != nil {
		t.Fatalf("redis buffer: %v", err)
	}
	if err := b.Ping(context.Background()); err != nil {
		t.Skipf("redis not reachable, skipping: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func dial(t *testing.T, url string) *amqp.Channel {
	t.Helper()
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Skipf("rabbitmq not reachable at %s: %v", url, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close(); _ = conn.Close() })
	return ch
}

// TestPublish_DeliversAndClearsBuffer: happy path. After Write the message is on
// the persist queue and the Redis buffer is empty.
func TestPublish_DeliversAndClearsBuffer(t *testing.T) {
	buf := newBuffer(t)
	admin := dial(t, adminURL())
	purgeQueue(t, admin)

	prodCh := dial(t, rabbitURL())
	pub, err := messaging.NewRabbitMQ(prodCh, exchange, 5*time.Second)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}

	s := sink.NewRabbitMQ(buf, pub, nil)
	msgID := "it-happy-" + time.Now().UTC().Format("150405.000")
	if err := s.Write(context.Background(), sampleEnvelope(msgID)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Message must land on the persist queue.
	msg := getOne(t, admin, 3*time.Second)
	if msg == nil {
		t.Fatal("no message delivered to persist queue")
	}
	var env map[string]any
	if err := json.Unmarshal(msg.Body, &env); err != nil {
		t.Fatalf("delivered body not JSON: %v", err)
	}
	if env["message_id"] != msgID {
		t.Fatalf("delivered message_id = %v, want %s", env["message_id"], msgID)
	}

	// Buffer must be empty for this message.
	assertNotBuffered(t, buf, msgID)
}

// TestBrokerDown_PersistsThenRetryRecovers: simulate a publish failure by using
// a closed channel, assert the entry stays in Redis, then a real RetryWorker
// pass republishes it.
func TestBrokerDown_PersistsThenRetryRecovers(t *testing.T) {
	buf := newBuffer(t)
	admin := dial(t, adminURL())
	purgeQueue(t, admin)

	msgID := "it-retry-" + time.Now().UTC().Format("150405.000")

	// Publisher on a deliberately closed channel -> publish fails, entry stays.
	badConn, err := amqp.Dial(rabbitURL())
	if err != nil {
		t.Skipf("rabbitmq not reachable: %v", err)
	}
	badCh, _ := badConn.Channel()
	badPub, err := messaging.NewRabbitMQ(badCh, exchange, 2*time.Second)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	_ = badCh.Close()
	_ = badConn.Close()

	s := sink.NewRabbitMQ(buf, badPub, nil)
	if err := s.Write(context.Background(), sampleEnvelope(msgID)); err != nil {
		t.Fatalf("write returned error (should swallow publish fail): %v", err)
	}
	assertBuffered(t, buf, msgID)

	// Now a healthy publisher + RetryWorker pass should drain it.
	goodCh := dial(t, rabbitURL())
	goodPub, err := messaging.NewRabbitMQ(goodCh, exchange, 5*time.Second)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	w := buffer.NewRetryWorker(buf, goodPub, time.Second, 0 /* minAge: retry immediately */, nil)
	// runPass is unexported; drive it via a short Run with a tiny interval.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Wait for delivery on the persist queue.
	if msg := getOne(t, admin, 4*time.Second); msg == nil {
		t.Fatal("retry did not republish to persist queue")
	}
	assertNotBuffered(t, buf, msgID)
}

func getOne(t *testing.T, ch *amqp.Channel, timeout time.Duration) *amqp.Delivery {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(persistQueue, true)
		if err != nil {
			t.Fatalf("queue get: %v", err)
		}
		if ok {
			return &msg
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func assertBuffered(t *testing.T, buf *buffer.Redis, msgID string) {
	t.Helper()
	msgs, err := buf.ScanOld(context.Background(), 0)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, m := range msgs {
		if m.MessageID == msgID {
			return
		}
	}
	t.Fatalf("message %s not found in buffer", msgID)
}

func assertNotBuffered(t *testing.T, buf *buffer.Redis, msgID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		msgs, err := buf.ScanOld(context.Background(), 0)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		found := false
		for _, m := range msgs {
			if m.MessageID == msgID {
				found = true
			}
		}
		if !found {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("message %s still buffered", msgID)
}
