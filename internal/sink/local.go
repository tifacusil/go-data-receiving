package sink

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// Stdout writes each envelope as a JSON line to standard output.
type Stdout struct{}

// NewStdout returns a stdout sink.
func NewStdout() *Stdout { return &Stdout{} }

// Write prints the serialized envelope followed by a newline.
func (*Stdout) Write(_ context.Context, env *parsing.Envelope) error {
	_, err := fmt.Println(parsing.Serialize(env))
	return err
}

// File writes each envelope to {dir}/{message_id}.json.
type File struct{ dir string }

// NewFile returns a file sink writing into dir (created if missing).
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	return &File{dir: dir}, nil
}

// Write persists the serialized envelope to a per-message file.
func (f *File) Write(_ context.Context, env *parsing.Envelope) error {
	path := filepath.Join(f.dir, env.MessageID+".json")
	return os.WriteFile(path, []byte(parsing.Serialize(env)), 0o644)
}

// Null discards every envelope.
type Null struct{}

// NewNull returns a no-op sink.
func NewNull() *Null { return &Null{} }

// Write does nothing.
func (*Null) Write(_ context.Context, _ *parsing.Envelope) error { return nil }
