package sink

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

func sampleEnvelope() *parsing.Envelope {
	return &parsing.Envelope{
		SchemaVersion: "1.0", MessageID: "550e8400-e29b-41d4-a716-446655440000",
		ProjectID: "shangtan", SensorID: "GXZM-DB03", SensorType: "gnss",
		Timestamp: "2026-05-02T14:30:00Z", ReceivedAt: "2026-05-02T14:30:12Z",
		SamplingIntervalSeconds: 3600,
		Payload: parsing.GnssPayload{
			DisplacementHorizontalMm: 13.57, CumulativeDisplacementMm: 1247.5, FixQuality: "rtk_fixed",
		},
	}
}

func TestFileSink_WritesPerMessageFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir)
	if err != nil {
		t.Fatalf("new file sink: %v", err)
	}
	if err := s.Write(context.Background(), sampleEnvelope()); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := filepath.Join(dir, "550e8400-e29b-41d4-a716-446655440000.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected envelope file: %v", err)
	}
	if string(data) != parsing.Serialize(sampleEnvelope()) {
		t.Fatalf("file content != serialized envelope: %s", data)
	}
}

func TestNullSink_NoError(t *testing.T) {
	if err := NewNull().Write(context.Background(), sampleEnvelope()); err != nil {
		t.Fatalf("null sink: %v", err)
	}
}
