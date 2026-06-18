package parsing

import "testing"

func TestDecodeHeader_Valid(t *testing.T) {
	msgID := [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
	hdr := buildHeader(msgID, "shangtan", "GXZM-DB03", 1714660200000, 3600)
	h, err := decodeHeader(hdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Message)
	}
	if h.messageID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("uuid = %s", h.messageID)
	}
	if h.projectID != "shangtan" || h.sensorID != "GXZM-DB03" {
		t.Fatalf("ids = %s / %s", h.projectID, h.sensorID)
	}
	if h.timestamp != "2024-05-02T14:30:00Z" {
		t.Fatalf("timestamp = %s, want 2024-05-02T14:30:00Z", h.timestamp)
	}
	if h.samplingInterval != 3600 {
		t.Fatalf("interval = %d", h.samplingInterval)
	}
}

func TestDecodeHeader_ProjectIDPattern(t *testing.T) {
	msgID := [16]byte{}
	// uppercase project id violates ^[a-z][a-z0-9_]{2,49}$
	hdr := buildHeader(msgID, "Shangtan", "S1", 0, 1)
	if _, err := decodeHeader(hdr); err == nil {
		t.Fatal("invalid project_id accepted")
	}
}

func TestDecodeHeader_RangeChecks(t *testing.T) {
	msgID := [16]byte{}
	// sampling interval 0 < 1
	if _, err := decodeHeader(buildHeader(msgID, "shangtan", "S1", 0, 0)); err == nil {
		t.Fatal("sampling_interval 0 accepted")
	}
	// sampling interval > 86400
	if _, err := decodeHeader(buildHeader(msgID, "shangtan", "S1", 0, 90000)); err == nil {
		t.Fatal("sampling_interval 90000 accepted")
	}
}

func TestDecodeHeader_TooShort(t *testing.T) {
	if _, err := decodeHeader(make([]byte, 10)); err == nil {
		t.Fatal("truncated header accepted")
	}
}

func TestParseStage_String(t *testing.T) {
	// Ordinals must match the C# enum (the ACK byte depends on them).
	cases := map[ParseStage]string{
		StageSyncBytes: "SyncBytes", StageVersion: "Version", StageSensorType: "SensorType",
		StageLength: "Length", StageCrc: "Crc", StageHeaderDecode: "HeaderDecode",
		StagePayloadDecode: "PayloadDecode", StageSchemaValidation: "SchemaValidation",
	}
	for stage, want := range cases {
		if got := stage.String(); got != want {
			t.Fatalf("stage %d = %s, want %s", int(stage), got, want)
		}
	}
	if int(StageSchemaValidation) != 7 {
		t.Fatalf("StageSchemaValidation ordinal = %d, want 7", int(StageSchemaValidation))
	}
}
