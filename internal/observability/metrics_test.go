package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("/metrics status = %d", rec.Code)
	}
	return rec.Body.String()
}

func TestMetrics_FramesAndLatency(t *testing.T) {
	m := NewMetrics()
	m.ObserveFrame("ok", "gnss", 0.002)
	m.ObserveFrame("ok", "gnss", 0.003)
	m.ObserveFrame("rejected", "unknown", 0.001)

	body := scrape(t, m)
	if !strings.Contains(body, `data_receiving_frames_total{result="ok",sensor_type="gnss"} 2`) {
		t.Fatalf("frames_total ok/gnss not 2:\n%s", body)
	}
	if !strings.Contains(body, `data_receiving_frames_total{result="rejected",sensor_type="unknown"} 1`) {
		t.Fatalf("frames_total rejected/unknown not 1")
	}
	if !strings.Contains(body, "data_receiving_frame_processing_seconds_bucket") {
		t.Fatal("latency histogram buckets missing")
	}
}

func TestMetrics_PublishedDropped(t *testing.T) {
	m := NewMetrics()
	m.IncPublished()
	m.IncPublished()
	m.IncDropped()

	body := scrape(t, m)
	if !strings.Contains(body, "data_receiving_messages_published_total 2") {
		t.Fatalf("published not 2:\n%s", body)
	}
	if !strings.Contains(body, "data_receiving_messages_dropped_total 1") {
		t.Fatal("dropped not 1")
	}
}

func TestMetrics_TCPConnections(t *testing.T) {
	m := NewMetrics()
	m.ConnOpened()
	m.ConnOpened()
	m.ConnClosed(1.5)

	body := scrape(t, m)
	if !strings.Contains(body, "data_receiving_tcp_connections_opened_total 2") {
		t.Fatalf("connections_opened_total not 2:\n%s", body)
	}
	// One closed of two opened => gauge should read 1.
	if !strings.Contains(body, "data_receiving_tcp_connections_active 1") {
		t.Fatalf("connections_active not 1 after one close:\n%s", body)
	}
	if !strings.Contains(body, "data_receiving_tcp_connection_duration_seconds_count 1") {
		t.Fatal("connection_duration_seconds observation missing")
	}
}

func TestMetrics_RuntimeCollectors(t *testing.T) {
	m := NewMetrics()
	body := scrape(t, m)
	// Process + Go collectors provide CPU and resident memory (CLAUDE.md §5).
	for _, want := range []string{"process_resident_memory_bytes", "process_cpu_seconds_total", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing runtime metric %q", want)
		}
	}
}
