// Package sink defines where parsed envelopes are written. The local sinks
// (stdout/file/null) need no external services; the rabbitmq sink is added in H3.
package sink

import (
	"context"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// Sink consumes a successfully parsed envelope. Ported from the C# ISink.
type Sink interface {
	Write(ctx context.Context, env *parsing.Envelope) error
}
