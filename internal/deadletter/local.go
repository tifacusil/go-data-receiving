// Package deadletter writes rejected frames to the local filesystem so a bad
// frame never blocks the pipeline. Port of the C# LocalDlxWriter.
package deadletter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// entry is the JSON record written per rejected frame. Field order and
// snake_case names match the C# DlxEntry record.
type entry struct {
	ReceivedAt   string `json:"received_at"`
	RawFrameHex  string `json:"raw_frame_hex"`
	Stage        string `json:"stage"`
	ErrorMessage string `json:"error_message"`
	AckStatus    int    `json:"ack_status"`
}

// Writer persists rejected frames as indented JSON files in a directory.
type Writer struct {
	dir string
	now func() time.Time // injectable for tests
	id  func() string    // injectable filename uuid (32 hex, no dashes)
}

// NewWriter creates the DLX directory and returns a Writer.
func NewWriter(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create dlx dir: %w", err)
	}
	return &Writer{
		dir: dir,
		now: time.Now,
		id:  randomHexID,
	}, nil
}

// randomHexID returns 16 random bytes as 32 hex chars, used to keep DLX
// filenames unique when two frames land in the same millisecond.
func randomHexID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error on supported platforms
	return hex.EncodeToString(b[:])
}

// Write serializes the rejected frame and its parse error to a new JSON file.
func (w *Writer) Write(_ context.Context, rawFrame []byte, perr *parsing.ParseError) error {
	now := w.now().UTC()
	e := entry{
		ReceivedAt:   now.Format("2006-01-02T15:04:05.000Z"),
		RawFrameHex:  strings.ToUpper(hex.EncodeToString(rawFrame)),
		Stage:        perr.Stage.String(),
		ErrorMessage: perr.Message,
		AckStatus:    ackStatusFor(perr.Stage),
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dlx entry: %w", err)
	}
	fileName := now.Format("20060102150405.000")
	fileName = strings.ReplaceAll(fileName, ".", "") + "_" + w.id() + ".json"
	return os.WriteFile(filepath.Join(w.dir, fileName), data, 0o644)
}

// ackStatusFor maps a parse stage to the dead-letter ack_status byte, matching
// the C# AckStatusFor (1=Crc, 4=SensorType, 3=SchemaValidation, 2=other).
func ackStatusFor(stage parsing.ParseStage) int {
	switch stage {
	case parsing.StageCrc:
		return 1
	case parsing.StageSensorType:
		return 4
	case parsing.StageSchemaValidation:
		return 3
	default:
		return 2
	}
}
