package deadletter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

func TestWrite_JSONAndFilename(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	// Pin the clock and uuid for a deterministic file.
	w.now = func() time.Time { return time.Date(2026, 5, 12, 14, 30, 0, 123_000_000, time.UTC) }
	w.id = func() string { return "abcdef0123456789abcdef0123456789" }

	frame := []byte{0xAA, 0x55, 0x01, 0x01, 0xFF, 0xFF}
	perr := &parsing.ParseError{Stage: parsing.StageCrc, Message: "CRC mismatch: expected 0x29B1 got 0xFFFF"}
	if err := w.Write(context.Background(), frame, perr); err != nil {
		t.Fatalf("write: %v", err)
	}

	wantName := "20260512143000123_abcdef0123456789abcdef0123456789.json"
	data, err := os.ReadFile(filepath.Join(dir, wantName))
	if err != nil {
		t.Fatalf("expected file %s: %v", wantName, err)
	}

	want := `{
  "received_at": "2026-05-12T14:30:00.123Z",
  "raw_frame_hex": "AA550101FFFF",
  "stage": "Crc",
  "error_message": "CRC mismatch: expected 0x29B1 got 0xFFFF",
  "ack_status": 1
}`
	if string(data) != want {
		t.Fatalf("dlx json mismatch\n got: %s\nwant: %s", data, want)
	}
}

func TestAckStatusFor(t *testing.T) {
	cases := map[parsing.ParseStage]int{
		parsing.StageCrc:              1,
		parsing.StageSensorType:       4,
		parsing.StageSchemaValidation: 3,
		parsing.StageHeaderDecode:     2,
	}
	for stage, want := range cases {
		if got := ackStatusFor(stage); got != want {
			t.Fatalf("ackStatusFor(%s) = %d, want %d", stage, got, want)
		}
	}
}
