package health

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServer_Run_ServesMetricsOnRealPort starts the actual Run() loop and hits
// /metrics over a real socket, reproducing the live-smoke path.
func TestServer_Run_ServesMetricsOnRealPort(t *testing.T) {
	// Pick a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	stub := &stubHandler{}
	s := NewServer(port, NewChecker(fakeTCP{up: true}, nil, nil), nil).WithMetrics(stub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Run(ctx) }()

	// Wait for the listener.
	waitListening(t, port)

	resp, err := http.Get("http://127.0.0.1:" + itoa(port) + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("/metrics status = %d body=%s", resp.StatusCode, body)
	}
	if !stub.hit {
		t.Fatal("stub metrics handler not hit")
	}
}

func waitListening(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+itoa(port), 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server never listened")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [6]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
