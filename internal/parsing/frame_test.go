package parsing

import "testing"

// validFrame returns a well-formed GNSS frame for mutation in tests.
func validFrame(t *testing.T) []byte {
	t.Helper()
	hdr := buildHeader([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		"shangtan", "GXZM-DB03", 1714660200000, 3600)
	body := gnssBody(12.34, -5.67, 2.10, 13.57, 1247.5, 0x04)
	return buildFrame(0x01, append(hdr, body...))
}

func TestValidateFrame_Valid(t *testing.T) {
	if r := validateFrame(validFrame(t)); !r.Success {
		t.Fatalf("valid frame rejected: %+v", r.Error)
	}
}

func TestValidateFrame_Errors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
		stage  ParseStage
	}{
		{"too short", func(_ []byte) []byte { return []byte{0xAA, 0x55, 0x01} }, StageSyncBytes},
		{"bad sync start", func(f []byte) []byte { f[0] = 0x00; return f }, StageSyncBytes},
		{"bad sync end", func(f []byte) []byte { f[len(f)-1] = 0x00; return f }, StageSyncBytes},
		{"bad version", func(f []byte) []byte { f[2] = 0x02; return f }, StageVersion},
		{"bad type", func(f []byte) []byte { f[3] = 0x09; return f }, StageSensorType},
		{"crc mismatch", func(f []byte) []byte { f[len(f)-3] ^= 0xFF; return f }, StageCrc},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validateFrame(tc.mutate(validFrame(t)))
			if r.Success {
				t.Fatal("expected failure, got success")
			}
			if r.Error.Stage != tc.stage {
				t.Fatalf("stage = %v, want %v (msg: %s)", r.Error.Stage, tc.stage, r.Error.Message)
			}
		})
	}
}

func TestValidateFrame_LengthMismatch(t *testing.T) {
	f := validFrame(t)
	// Corrupt declared length so it no longer matches the real frame size.
	f[5]++
	r := validateFrame(f)
	if r.Success || r.Error.Stage != StageLength {
		t.Fatalf("want Length failure, got success=%v err=%+v", r.Success, r.Error)
	}
}
