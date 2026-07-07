package buffer

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeBuffer holds messages in memory keyed by message_id.
type fakeBuffer struct {
	msgs   map[string]BufferedMessage
	delErr error
}

func newFakeBuffer(msgs ...BufferedMessage) *fakeBuffer {
	m := map[string]BufferedMessage{}
	for _, msg := range msgs {
		m[msg.MessageID] = msg
	}
	return &fakeBuffer{msgs: m}
}

func (f *fakeBuffer) Set(_ context.Context, id, rk, json string) error {
	f.msgs[id] = BufferedMessage{MessageID: id, RoutingKey: rk, EnvelopeJSON: json}
	return nil
}
func (f *fakeBuffer) Delete(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.msgs, id)
	return nil
}
func (f *fakeBuffer) ScanOld(_ context.Context, _ time.Duration) ([]BufferedMessage, error) {
	out := make([]BufferedMessage, 0, len(f.msgs))
	for _, m := range f.msgs {
		out = append(out, m)
	}
	return out, nil
}
func (f *fakeBuffer) Count(_ context.Context) (int64, error) { return int64(len(f.msgs)), nil }

type fakePub struct {
	err   error
	calls int
}

func (p *fakePub) PublishJSON(_ context.Context, _, _, _ string) error {
	p.calls++
	return p.err
}

type fakeGauge struct {
	lastValue int64
	calls     int
}

func (g *fakeGauge) SetRedisBufferPending(n int64) {
	g.lastValue = n
	g.calls++
}

func TestRetryWorker_Pass_RepublishesAndDeletes(t *testing.T) {
	buf := newFakeBuffer(BufferedMessage{MessageID: "m1", RoutingKey: "sensors.shangtan.gnss"})
	pub := &fakePub{}
	w := NewRetryWorker(buf, pub, time.Second, time.Minute, nil, nil)

	w.runPass(context.Background())

	if pub.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", pub.calls)
	}
	if len(buf.msgs) != 0 {
		t.Fatalf("buffer not drained after successful retry: %v", buf.msgs)
	}
}

func TestRetryWorker_Pass_PublishFails_KeepsEntry(t *testing.T) {
	buf := newFakeBuffer(BufferedMessage{MessageID: "m1", RoutingKey: "sensors.shangtan.gnss"})
	pub := &fakePub{err: errors.New("still down")}
	w := NewRetryWorker(buf, pub, time.Second, time.Minute, nil, nil)

	w.runPass(context.Background())

	if len(buf.msgs) != 1 {
		t.Fatalf("entry should remain when publish fails, msgs=%v", buf.msgs)
	}
}

func TestRetryWorker_Pass_ReportsBacklogGauge(t *testing.T) {
	buf := newFakeBuffer(
		BufferedMessage{MessageID: "m1", RoutingKey: "sensors.shangtan.gnss"},
		BufferedMessage{MessageID: "m2", RoutingKey: "sensors.shangtan.gnss"},
	)
	pub := &fakePub{err: errors.New("still down")} // keep both entries buffered
	gauge := &fakeGauge{}
	w := NewRetryWorker(buf, pub, time.Second, time.Minute, nil, gauge)

	w.runPass(context.Background())

	if gauge.calls != 1 {
		t.Fatalf("gauge calls = %d, want 1", gauge.calls)
	}
	if gauge.lastValue != 2 {
		t.Fatalf("gauge value = %d, want 2", gauge.lastValue)
	}
}

func TestRetryWorker_Run_StopsOnContextCancel(t *testing.T) {
	buf := newFakeBuffer()
	w := NewRetryWorker(buf, &fakePub{}, 10*time.Millisecond, time.Minute, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}
