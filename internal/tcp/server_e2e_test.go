package tcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AngelloFD/go-data-receiving/internal/deadletter"
	"github.com/AngelloFD/go-data-receiving/internal/parsing"
	"github.com/AngelloFD/go-data-receiving/internal/schema"
	"github.com/AngelloFD/go-data-receiving/internal/sink"
)

// --- frame construction helpers ---

func f32(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func gnssFrame(t *testing.T, corruptCRC bool) []byte {
	t.Helper()
	payload := make([]byte, 0, 64)
	payload = append(payload, make([]byte, 16)...) // uuid
	payload = append(payload, byte(len("shangtan")))
	payload = append(payload, "shangtan"...)
	payload = append(payload, byte(len("GXZM-DB03")))
	payload = append(payload, "GXZM-DB03"...)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, 1714660200000)
	payload = append(payload, ts...)
	si := make([]byte, 4)
	binary.BigEndian.PutUint32(si, 3600)
	payload = append(payload, si...)
	// gnss body: 5 floats + fix byte
	payload = append(payload, f32(12.34)...)
	payload = append(payload, f32(-5.67)...)
	payload = append(payload, f32(2.10)...)
	payload = append(payload, f32(13.57)...)
	payload = append(payload, f32(1247.5)...)
	payload = append(payload, 0x04)

	frame := []byte{0xAA, 0x55, 0x01, 0x01, byte(len(payload) >> 8), byte(len(payload))}
	frame = append(frame, payload...)
	crc := parsing.CRC16(frame[2:])
	if corruptCRC {
		crc ^= 0xFFFF
	}
	frame = append(frame, byte(crc>>8), byte(crc))
	frame = append(frame, 0x55, 0xAA)
	return frame
}

// startServer binds an ephemeral port, runs the server, returns its address and
// a cancel func.
func startServer(t *testing.T, out Sink, dlx DeadLetter) (string, context.CancelFunc) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	parser := parsing.NewParser(schema.New())
	srv := NewServer(0, parser, out, dlx, NewStatus(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.serve(ctx, ln)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	})
	return ln.Addr().String(), cancel
}

func readAck(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	ack := make([]byte, 5)
	if _, err := io.ReadFull(conn, ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	return ack
}

func TestE2E_ValidFrame_SinkAndAck(t *testing.T) {
	outDir := t.TempDir()
	out, err := sink.NewFile(outDir)
	if err != nil {
		t.Fatalf("file sink: %v", err)
	}
	dlx, err := deadletter.NewWriter(t.TempDir())
	if err != nil {
		t.Fatalf("dlx: %v", err)
	}

	addr, _ := startServer(t, out, dlx)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write(gnssFrame(t, false)); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	ack := readAck(t, conn)
	if !bytes.Equal(ack, []byte{0xAA, 0x55, 0x00, 0x00, 0xAA}) {
		t.Fatalf("ack = % X, want AA 55 00 00 AA", ack)
	}

	// Sink must have produced exactly one envelope file.
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 envelope file, got %d", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	if !bytes.Contains(data, []byte(`"sensor_type":"gnss"`)) {
		t.Fatalf("envelope content unexpected: %s", data)
	}
}

func TestE2E_CorruptCRC_DlxAndAck(t *testing.T) {
	dlxDir := t.TempDir()
	dlx, err := deadletter.NewWriter(dlxDir)
	if err != nil {
		t.Fatalf("dlx: %v", err)
	}
	addr, _ := startServer(t, sink.NewNull(), dlx)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write(gnssFrame(t, true)); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	ack := readAck(t, conn)
	// CRC failure -> status 0x01, code = StageCrc ordinal (4).
	if ack[2] != 0x01 {
		t.Fatalf("ack status = 0x%02X, want 0x01", ack[2])
	}
	if ack[3] != byte(parsing.StageCrc) {
		t.Fatalf("ack code = %d, want %d", ack[3], int(parsing.StageCrc))
	}

	entries, _ := os.ReadDir(dlxDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 dlx file, got %d", len(entries))
	}
}

// countingObserver records Observer calls without needing a real
// Prometheus registry; used to assert the accept-loop wiring itself (that
// handleConn actually calls ConnOpened/ConnClosed), independent of the metric
// storage logic already covered in the observability package.
type countingObserver struct {
	opened, closed int
	lastDuration   float64
}

func (c *countingObserver) ObserveFrame(string, string, float64) {}
func (c *countingObserver) IncPublished()                        {}
func (c *countingObserver) IncDropped()                          {}
func (c *countingObserver) ConnOpened()                          { c.opened++ }
func (c *countingObserver) ConnClosed(seconds float64) {
	c.closed++
	c.lastDuration = seconds
}

func TestE2E_TCPConnection_ObservedOpenAndClose(t *testing.T) {
	dlx, err := deadletter.NewWriter(t.TempDir())
	if err != nil {
		t.Fatalf("dlx: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	parser := parsing.NewParser(schema.New())
	obs := &countingObserver{}
	srv := NewServer(0, parser, sink.NewNull(), dlx, NewStatus(), nil).WithObservability(obs, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close() // clean close, no frame sent

	// handleConn runs in its own goroutine; give it a moment to observe the close.
	deadline := time.Now().Add(2 * time.Second)
	for obs.closed == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if obs.opened != 1 {
		t.Fatalf("ConnOpened calls = %d, want 1", obs.opened)
	}
	if obs.closed != 1 {
		t.Fatalf("ConnClosed calls = %d, want 1", obs.closed)
	}
	if obs.lastDuration < 0 {
		t.Fatalf("connection duration = %v, want >= 0", obs.lastDuration)
	}
}
