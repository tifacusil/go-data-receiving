package observability

import (
	"log/slog"
	"os"
)

// NewLogger builds the structured JSON logger required by CLAUDE.md §5: every
// line carries service, instance_id and version. The standard slog JSON handler
// already emits time/level/msg; per-event fields (message_id/trace_id, event)
// are added at the call sites.
func NewLogger(service, instanceID, version string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", service),
		slog.String("instance_id", instanceID),
		slog.String("version", version),
	)
}
