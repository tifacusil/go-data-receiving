// Package observability holds the cross-cutting concerns the C# baseline lacked:
// Prometheus metrics, structured logging and OpenTelemetry tracing (CLAUDE.md
// §5). It is a non-intrusive layer — it observes the pipeline without changing
// the byte-for-byte behaviour that the Go-vs-C# performance comparison depends on.
package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the service collectors registered in a private registry (no
// global state, so tests stay isolated).
type Metrics struct {
	registry *prometheus.Registry

	framesTotal  *prometheus.CounterVec
	frameLatency prometheus.Histogram
	published    prometheus.Counter
	dropped      prometheus.Counter

	// TCP connection tracking: this service's transport is TCP, not HTTP, so
	// there is no request/response cycle to instrument generically like the
	// other Go services' http_request_duration_seconds. These are the TCP
	// equivalent of that coverage (accept rate, concurrent load, session length).
	connsOpenedTotal prometheus.Counter
	connsActive      prometheus.Gauge
	connDuration     prometheus.Histogram
}

// NewMetrics builds and registers the collectors, including the Go runtime and
// process collectors that expose process_resident_memory_bytes and
// process_cpu_seconds_total (CLAUDE.md §2.1/§5).
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,
		framesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "data_receiving_frames_total",
			Help: "Total DTU frames processed, by result (ok|rejected) and sensor_type.",
		}, []string{"result", "sensor_type"}),
		frameLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "data_receiving_frame_processing_seconds",
			Help: "Wall-clock time to parse and sink a frame.",
			// Buckets span sub-millisecond to ~1s; sufficient to derive p50/p90/p95/p99.
			Buckets: []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "data_receiving_messages_published_total",
			Help: "Envelopes confirmed published to RabbitMQ.",
		}),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "data_receiving_messages_dropped_total",
			Help: "Frames rejected and written to the dead-letter directory.",
		}),
		connsOpenedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "data_receiving_tcp_connections_opened_total",
			Help: "Total TCP connections accepted.",
		}),
		connsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "data_receiving_tcp_connections_active",
			Help: "TCP connections currently open.",
		}),
		connDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "data_receiving_tcp_connection_duration_seconds",
			Help: "Wall-clock time a TCP connection stayed open.",
			// DTUs typically hold a connection open for a session of frames;
			// buckets span sub-second (bad connect) to tens of minutes.
			Buckets: []float64{0.5, 1, 5, 15, 30, 60, 300, 600, 1800, 3600},
		}),
	}

	reg.MustRegister(
		m.framesTotal, m.frameLatency, m.published, m.dropped,
		m.connsOpenedTotal, m.connsActive, m.connDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// ObserveFrame records the outcome and latency of one frame.
func (m *Metrics) ObserveFrame(result, sensorType string, seconds float64) {
	m.framesTotal.WithLabelValues(result, sensorType).Inc()
	m.frameLatency.Observe(seconds)
}

// IncPublished increments the confirmed-publish counter.
func (m *Metrics) IncPublished() { m.published.Inc() }

// IncDropped increments the dead-lettered counter.
func (m *Metrics) IncDropped() { m.dropped.Inc() }

// ConnOpened records a new accepted TCP connection.
func (m *Metrics) ConnOpened() {
	m.connsOpenedTotal.Inc()
	m.connsActive.Inc()
}

// ConnClosed records a TCP connection closing after having stayed open for
// seconds. Pairs with ConnOpened; the gauge only ever moves through this pair.
func (m *Metrics) ConnClosed(seconds float64) {
	m.connsActive.Dec()
	m.connDuration.Observe(seconds)
}

// Handler returns the /metrics HTTP handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
