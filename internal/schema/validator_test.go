package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_GoodEnvelope(t *testing.T) {
	v := New()
	good := `{"schema_version":"1.0","message_id":"550e8400-e29b-41d4-a716-446655440000",` +
		`"project_id":"shangtan","sensor_id":"GXZM-DB03","sensor_type":"gnss",` +
		`"timestamp":"2026-05-02T14:30:00Z","received_at":"2026-05-02T14:30:12Z",` +
		`"sampling_interval_seconds":3600,"payload":{` +
		`"displacement_x_mm":12.34,"displacement_y_mm":-5.67,"displacement_z_mm":2.1,` +
		`"displacement_horizontal_mm":13.57,"cumulative_displacement_mm":1247.5,` +
		`"fix_quality":"rtk_fixed"}}`
	if ok, msg := v.Validate(good, "gnss"); !ok {
		t.Fatalf("valid envelope rejected: %s", msg)
	}
}

func TestValidate_RejectsBadPayload(t *testing.T) {
	v := New()
	// fix_quality not in enum
	bad := `{"schema_version":"1.0","message_id":"550e8400-e29b-41d4-a716-446655440000",` +
		`"project_id":"shangtan","sensor_id":"GXZM-DB03","sensor_type":"gnss",` +
		`"timestamp":"2026-05-02T14:30:00Z","received_at":"2026-05-02T14:30:12Z",` +
		`"sampling_interval_seconds":3600,"payload":{` +
		`"displacement_x_mm":12.34,"displacement_y_mm":-5.67,"displacement_z_mm":2.1,` +
		`"displacement_horizontal_mm":13.57,"cumulative_displacement_mm":1247.5,` +
		`"fix_quality":"banana"}}`
	if ok, _ := v.Validate(bad, "gnss"); ok {
		t.Fatal("invalid fix_quality accepted")
	}
}

func TestValidate_RejectsAdditionalProperties(t *testing.T) {
	v := New()
	bad := `{"schema_version":"1.0","message_id":"550e8400-e29b-41d4-a716-446655440000",` +
		`"project_id":"shangtan","sensor_id":"S1","sensor_type":"strain",` +
		`"timestamp":"2026-05-02T14:00:00Z","received_at":"2026-05-02T14:00:05Z",` +
		`"sampling_interval_seconds":3600,"extra":"nope","payload":{"strain_microstrain":1.0}}`
	if ok, _ := v.Validate(bad, "strain"); ok {
		t.Fatal("additionalProperties not rejected at envelope level")
	}
}

func TestValidate_RejectsBadJSON(t *testing.T) {
	v := New()
	if ok, msg := v.Validate("{not json", "gnss"); ok || msg != "Failed to parse envelope JSON" {
		t.Fatalf("bad JSON not rejected: ok=%v msg=%q", ok, msg)
	}
}

// TestContract_ExamplesValidate ensures every contract example in
// contracts/examples validates against the embedded schemas (CLAUDE.md §3.2).
func TestContract_ExamplesValidate(t *testing.T) {
	v := New()
	dir := filepath.Join("..", "..", "contracts", "examples")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no example files found")
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var env struct {
				SensorType string `json:"sensor_type"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("parse sensor_type: %v", err)
			}
			if ok, msg := v.Validate(string(raw), env.SensorType); !ok {
				t.Fatalf("example %s failed schema: %s", e.Name(), msg)
			}
		})
	}
}
