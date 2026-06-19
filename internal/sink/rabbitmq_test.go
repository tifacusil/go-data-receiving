package sink

import (
	"context"
	"errors"
	"testing"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// fakeBuffer records Set/Delete calls and tracks which message_ids remain.
type fakeBuffer struct {
	stored   map[string]string // messageID -> routingKey
	setErr   error
	delErr   error
	setCalls int
	delCalls int
}

func newFakeBuffer() *fakeBuffer { return &fakeBuffer{stored: map[string]string{}} }

func (f *fakeBuffer) Set(_ context.Context, messageID, routingKey, _ string) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	f.stored[messageID] = routingKey
	return nil
}

func (f *fakeBuffer) Delete(_ context.Context, messageID string) error {
	f.delCalls++
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.stored, messageID)
	return nil
}

// fakePublisher captures publishes and can be made to fail.
type fakePublisher struct {
	pubErr error
	calls  int
	lastRK string
}

func (p *fakePublisher) PublishJSON(_ context.Context, routingKey, _, _ string) error {
	p.calls++
	p.lastRK = routingKey
	return p.pubErr
}

func env() *parsing.Envelope {
	return &parsing.Envelope{
		SchemaVersion: "1.0", MessageID: "m1", ProjectID: "shangtan", SensorType: "gnss",
		Payload: parsing.GnssPayload{FixQuality: "rtk_fixed"},
	}
}

func TestRabbitMQSink_HappyPath_DeletesAfterConfirm(t *testing.T) {
	buf := newFakeBuffer()
	pub := &fakePublisher{}
	s := NewRabbitMQ(buf, pub, nil)

	if err := s.Write(context.Background(), env()); err != nil {
		t.Fatalf("write: %v", err)
	}
	if pub.calls != 1 || pub.lastRK != "sensors.shangtan.gnss" {
		t.Fatalf("publish calls=%d rk=%q", pub.calls, pub.lastRK)
	}
	if len(buf.stored) != 0 {
		t.Fatalf("buffer not cleared after confirm: %v", buf.stored)
	}
	if buf.setCalls != 1 || buf.delCalls != 1 {
		t.Fatalf("set=%d del=%d, want 1/1", buf.setCalls, buf.delCalls)
	}
}

func TestRabbitMQSink_PublishFails_KeepsInBuffer_NoError(t *testing.T) {
	buf := newFakeBuffer()
	pub := &fakePublisher{pubErr: errors.New("broker down")}
	s := NewRabbitMQ(buf, pub, nil)

	// Publish failure must NOT propagate (matches C#: still ACK success).
	if err := s.Write(context.Background(), env()); err != nil {
		t.Fatalf("expected nil error on publish failure, got %v", err)
	}
	if len(buf.stored) != 1 {
		t.Fatalf("entry must stay buffered for retry, stored=%v", buf.stored)
	}
	if buf.delCalls != 0 {
		t.Fatalf("delete must not be called on publish failure, del=%d", buf.delCalls)
	}
}

func TestRabbitMQSink_BufferSetFails_ReturnsError(t *testing.T) {
	buf := newFakeBuffer()
	buf.setErr = errors.New("redis down")
	pub := &fakePublisher{}
	s := NewRabbitMQ(buf, pub, nil)

	// If buffering fails the durability guarantee is broken -> error, no publish.
	if err := s.Write(context.Background(), env()); err == nil {
		t.Fatal("expected error when buffer.Set fails")
	}
	if pub.calls != 0 {
		t.Fatalf("must not publish when buffering failed, calls=%d", pub.calls)
	}
}
