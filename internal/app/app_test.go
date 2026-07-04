package app

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// In non-rabbitmq modes (file/null/stdout) the health/metrics/docs server must
// still run: it used to be wired only inside the rabbitmq branch, so /health and
// /metrics were dead outside that mode even though metrics were recorded.
func TestRun_HealthAndMetricsServeInStdoutMode(t *testing.T) {
	tcpPort := freePort(t)
	healthPort := freePort(t)

	t.Setenv("RECEIVER__OUTPUTMODE", "stdout")
	t.Setenv("RECEIVER__PORT", strconv.Itoa(tcpPort))
	t.Setenv("RECEIVER__CONSUL__HEALTHPORT", strconv.Itoa(healthPort))
	t.Setenv("RECEIVER__CONSUL__ENABLED", "false")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, "test") }()
	defer func() {
		cancel()
		<-done
	}()

	base := "http://127.0.0.1:" + strconv.Itoa(healthPort)
	waitUp(t, base+"/health")

	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", resp.StatusCode)
	}
}

func waitUp(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never came up", url)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
