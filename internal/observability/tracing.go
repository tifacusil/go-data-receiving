package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OpenTelemetry tracer for the frame pipeline. Without a
// configured exporter the global provider is a no-op, so this stays cheap and
// safe to call even when no collector is running (CLAUDE.md §5: tracing
// recommended, message_id used as the correlation id). A real OTLP exporter is
// wired from config in H7.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer returns a Tracer backed by the global OTel provider.
func NewTracer() *Tracer {
	return &Tracer{tracer: otel.Tracer("data-receiving")}
}

// StartFrame opens a span for processing one frame, correlated by message_id.
// The returned func ends the span.
func (t *Tracer) StartFrame(ctx context.Context, sensorType, messageID string) (context.Context, func()) {
	ctx, span := t.tracer.Start(ctx, "frame.process",
		trace.WithAttributes(
			attribute.String("sensor_type", sensorType),
			attribute.String("message_id", messageID),
		),
	)
	return ctx, func() { span.End() }
}
