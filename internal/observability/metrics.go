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
	}

	reg.MustRegister(
		m.framesTotal, m.frameLatency, m.published, m.dropped,
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

// Handler returns the /metrics HTTP handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
